package scaffold

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func conflictContentID(base, incoming, local []byte) string {
	h := sha256.New()
	var size [8]byte
	for _, data := range [][]byte{base, incoming, local} {
		binary.BigEndian.PutUint64(size[:], uint64(len(data)))
		_, _ = h.Write(size[:])
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *updateRun) recordConflict(f managedFile, local, merged []byte, sources []pendingSource) error {
	baseRel := f.Rel
	for _, source := range sources {
		if source.LocalSHA256 == contentDigest(local) {
			baseRel = source.Rel
			break
		}
	}
	base, _, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+baseRel)
	if err != nil {
		return err
	}
	candidate, err := r.writeConflictCandidate(conflictContentID(base, f.Data, local), merged, f.Data)
	if err != nil {
		return err
	}
	p := pendingUpdate{Stage: "conflict", IncomingSHA256: contentDigest(f.Data), LocalSHA256: contentDigest(local), Candidate: candidate, Sources: sources}
	old, existed := r.state.Pending[managedMapKey(r.state.Pending, f.Rel)]
	r.state.Pending[managedMapKey(r.state.Pending, f.Rel)] = p
	if err := r.saveState(r.state); err != nil {
		if existed {
			r.state.Pending[managedMapKey(r.state.Pending, f.Rel)] = old
		} else {
			delete(r.state.Pending, managedMapKey(r.state.Pending, f.Rel))
		}
		return fmt.Errorf("scaffold: saving conflict for %s (candidate %s): %w", f.Rel, candidate, err)
	}
	r.reportPendingConflict(f.Rel, p)
	return nil
}

func (r *updateRun) writeConflictCandidate(id string, merged, incoming []byte) (string, error) {
	ignore, err := projectFilePath(r.opts.Root, ".mulix/.update-conflicts/.gitignore")
	if err != nil {
		return "", err
	}
	if err := ensureAuxiliaryIgnore(ignore); err != nil {
		return "", err
	}
	dir := path.Join(".mulix/.update-conflicts", id)
	probe, err := projectFilePath(r.opts.Root, dir+"/.probe")
	if err != nil {
		return "", err
	}
	if err := os.Mkdir(filepath.Dir(probe), 0o755); err != nil && !os.IsExist(err) {
		return "", err
	}
	matches := true
	for _, part := range []struct {
		name string
		data []byte
	}{{"merged.md", merged}, {"incoming.md", incoming}} {
		existing, exists, err := readInstalledFile(r.opts.Root, path.Join(dir, part.name))
		if err != nil {
			return "", err
		}
		if exists && !bytes.Equal(existing, part.data) {
			matches = false
		}
	}
	if !matches {
		full, err := os.MkdirTemp(filepath.Dir(ignore), id+"-*")
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(r.opts.Root, full)
		if err != nil {
			return "", err
		}
		dir = filepath.ToSlash(relative)
	}
	for _, part := range []struct {
		name string
		data []byte
	}{{"merged.md", merged}, {"incoming.md", incoming}} {
		rel := path.Join(dir, part.name)
		existing, exists, err := readInstalledFile(r.opts.Root, rel)
		if err != nil {
			return "", err
		}
		if exists && bytes.Equal(existing, part.data) {
			continue
		}
		if exists {
			return "", fmt.Errorf("scaffold: candidate %s changed during preparation; retained", rel)
		}
		full, err := projectFilePath(r.opts.Root, rel)
		if err != nil {
			return "", err
		}
		if err := r.write(full, part.data, 0o644); err != nil {
			return "", fmt.Errorf("scaffold: writing candidate %s: %w", rel, err)
		}
	}
	return path.Join(dir, "merged.md"), nil
}

func (r *updateRun) reportPendingConflict(rel string, p pendingUpdate) {
	r.result.Conflicted = append(r.result.Conflicted, rel+" (candidate: "+p.Candidate+")")
}

func hasToolConflictMarkers(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "<<<<<<< incoming (mulix)" || line == ">>>>>>> local" {
			return true
		}
	}
	return false
}

func (r *updateRun) acceptanceContent(rel string, incoming managedFile) (pendingUpdate, []byte, error) {
	p, ok := r.state.Pending[managedMapKey(r.state.Pending, rel)]
	if !ok {
		return p, nil, fmt.Errorf("scaffold: %s has no pending update", rel)
	}
	if managedPathIdentity(incoming.Rel) != managedPathIdentity(rel) || p.Stage != "conflict" {
		return p, nil, fmt.Errorf("scaffold: %s is not an ordinary conflict acceptance", rel)
	}
	if p.IncomingSHA256 != contentDigest(incoming.Data) {
		return p, nil, fmt.Errorf("scaffold: %s pending vendor differs from this binary; resolve with the matching binary or explicitly use --force", rel)
	}
	local, exists, err := readInstalledFile(r.opts.Root, rel)
	if err != nil {
		return p, nil, err
	}
	if !exists {
		return p, nil, fmt.Errorf("scaffold: resolved target %s is missing; place the resolution there before --accept", rel)
	}
	if hasToolConflictMarkers(local) {
		return p, nil, fmt.Errorf("scaffold: %s still contains mulix conflict markers", rel)
	}
	return p, local, nil
}

func (r *updateRun) acceptPending(rel string, incoming managedFile) (err error) {
	var completed []string
	stage := "acceptance baseline"
	defer func() { r.recordPartial(rel, completed, stage, err) }()
	if p, ok := r.state.Pending[managedMapKey(r.state.Pending, rel)]; ok && p.Stage == "accepted-cleanup" {
		if err := r.finishEntryMigration(rel, p); err != nil {
			r.blockEntry(rel, err)
			return err
		}
		r.result.Accepted = append(r.result.Accepted, rel)
		r.result.Migrated = append(r.result.Migrated, rel)
		return r.updateFile(incoming)
	}
	if err := r.validateAcceptance(rel, incoming); err != nil {
		r.blockEntry(rel, err)
		return err
	}
	p, local, err := r.acceptanceContent(rel, incoming)
	if err != nil {
		return err
	}
	if len(p.Sources) > 0 {
		if err := r.startEntryMigration(incoming, local, local, p.Sources, true); err != nil {
			r.blockEntry(rel, err)
			return err
		}
		r.result.Accepted = append(r.result.Accepted, rel)
		r.result.Migrated = append(r.result.Migrated, rel)
		return nil
	}
	if err := r.preserve(rel); err != nil {
		return err
	}
	full, err := projectFilePath(r.opts.Root, ".mulix/.installed/"+rel)
	if err != nil {
		return err
	}
	if err := r.write(full, incoming.Data, 0o644); err != nil {
		return fmt.Errorf("scaffold: accepting vendor baseline for %s: %w", rel, err)
	}
	completed = append(completed, "vendor baseline written")
	stage = "acceptance metadata"
	old, recorded := r.state.Files[managedMapKey(r.state.Files, rel)]
	r.state.Files[managedMapKey(r.state.Files, rel)] = vendorRecord{SHA256: contentDigest(incoming.Data)}
	delete(r.state.Pending, managedMapKey(r.state.Pending, rel))
	if err := r.saveState(r.state); err != nil {
		r.state.Pending[managedMapKey(r.state.Pending, rel)] = p
		if recorded {
			r.state.Files[managedMapKey(r.state.Files, rel)] = old
		} else {
			delete(r.state.Files, managedMapKey(r.state.Files, rel))
		}
		return fmt.Errorf("scaffold: saving acceptance for %s: %w", rel, err)
	}
	r.result.Accepted = append(r.result.Accepted, rel)
	return nil
}

func (r *updateRun) clearPending(rel string) error {
	p, ok := r.state.Pending[managedMapKey(r.state.Pending, rel)]
	if !ok {
		return nil
	}
	delete(r.state.Pending, managedMapKey(r.state.Pending, rel))
	if err := r.saveState(r.state); err != nil {
		r.state.Pending[managedMapKey(r.state.Pending, rel)] = p
		return fmt.Errorf("scaffold: saving pending completion for %s: %w", rel, err)
	}
	return nil
}

// Pending decisions precede even missing-L installation or current-L adoption.
func (r *updateRun) handlePending(f managedFile) (bool, error) {
	p, ok := r.state.Pending[managedMapKey(r.state.Pending, f.Rel)]
	if !ok {
		return false, nil
	}
	if p.Stage != "conflict" || len(p.Sources) > 0 {
		r.result.Blocked = append(r.result.Blocked, f.Rel+" (entry migration cleanup pending; sources retained)")
		return true, nil
	}
	if r.opts.Force {
		if err := r.preserve(f.Rel); err != nil {
			return true, err
		}
		if err := r.commitVendor(f.Rel, f.Data, f.Data, f.Mode()); err != nil {
			return true, err
		}
		if err := r.clearPending(f.Rel); err != nil {
			r.recordPartial(f.Rel, []string{"current vendor installed"}, "pending metadata", err)
			return true, err
		}
		r.result.Updated = append(r.result.Updated, f.Rel+" (forced)")
		return true, nil
	}
	if p.IncomingSHA256 != contentDigest(f.Data) {
		r.result.Blocked = append(r.result.Blocked, f.Rel+" (pending vendor differs from this binary; candidate retained at "+p.Candidate+")")
	} else {
		r.reportPendingConflict(f.Rel, p)
	}
	return true, nil
}
