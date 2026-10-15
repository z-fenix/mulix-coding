package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type entryBlockedError struct{ reason string }

func (e *entryBlockedError) Error() string { return e.reason }
func entryProblem(format string, args ...any) error {
	return &entryBlockedError{reason: fmt.Sprintf(format, args...)}
}

type entrySource struct {
	snapshot                    pendingSource
	local, base                 []byte
	hasLocal, hasBase, pristine bool
}

func skillName(data []byte) string {
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") != "---" {
			continue
		}
		var meta struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &meta); err != nil {
			return ""
		}
		return meta.Name
	}
	return ""
}

// Only the explicit alias name scalar changes. Body bytes, comments and line
// endings are kept; unusual YAML forms are left for conflict/manual handling.
func canonicalEntryBytes(data []byte) []byte {
	name := skillName(data)
	alias := false
	for _, old := range entryAliases {
		if name == old {
			alias = true
		}
	}
	if !alias {
		return data
	}
	lines := strings.Split(string(data), "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			break
		}
		trimmed := strings.TrimLeft(lines[i], " \t")
		if !strings.HasPrefix(trimmed, "name:") {
			continue
		}
		scalar := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(trimmed, "name:"), "#", 2)[0])
		replacement := entrySkillName
		switch scalar {
		case name:
		case "\"" + name + "\"":
			replacement = "\"" + entrySkillName + "\""
		case "'" + name + "'":
			replacement = "'" + entrySkillName + "'"
		default:
			continue
		}
		lines[i] = strings.Replace(lines[i], scalar, replacement, 1)
		break
	}
	return []byte(strings.Join(lines, "\n"))
}

func (r *updateRun) blockEntry(target string, err error) (bool, bool, error) {
	var blocked *entryBlockedError
	if !errors.As(err, &blocked) {
		return true, false, err
	}
	r.result.Blocked = append(r.result.Blocked, target+" ("+err.Error()+")")
	return true, false, nil
}

func (r *updateRun) collectEntrySources(host Host, p pendingUpdate) ([]entrySource, error) {
	referenced := map[string]bool{}
	for _, source := range p.Sources {
		referenced[managedPathIdentity(source.Rel)] = true
	}
	var sources []entrySource
	for _, alias := range entryAliases {
		rel := entryRel(host, alias)
		local, hasLocal, err := readInstalledFile(r.opts.Root, rel)
		if err != nil {
			return nil, err
		}
		base, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+rel)
		if err != nil {
			return nil, err
		}
		if !hasLocal && !hasBase && !referenced[managedPathIdentity(rel)] {
			continue
		}
		source := entrySource{local: local, base: base, hasLocal: hasLocal, hasBase: hasBase,
			snapshot: pendingSource{Rel: rel, LocalSHA256: contentDigest(local)}}
		if hasBase {
			source.snapshot.BaselineSHA256 = contentDigest(base)
		}
		knownLocal := hasLocal && legacyVendorMatch(rel, local)
		trustedBase := hasBase && r.state.trustedVendor(rel, base, nil)
		source.pristine = knownLocal || (trustedBase && (!hasLocal || bytes.Equal(local, base)))
		if !r.opts.Force && !knownLocal && !trustedBase {
			return nil, entryProblem("source %s has no trusted vendor ancestor; retained", rel)
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func (r *updateRun) entryReady(rel string) (bool, error) {
	if _, pending := r.state.Pending[managedMapKey(r.state.Pending, rel)]; pending {
		return false, nil
	}
	for _, blocked := range r.result.Blocked {
		if strings.HasPrefix(blocked, rel+" (") {
			return false, nil
		}
	}
	local, exists, err := readInstalledFile(r.opts.Root, rel)
	if err != nil {
		return false, err
	}
	return exists && skillName(local) == entrySkillName && !hasToolConflictMarkers(local), nil
}

func (r *updateRun) migrateEntry(host Host, incoming managedFile) (bool, bool, error) {
	target := entryRel(host, entrySkillName)
	if incoming.Rel != target || skillName(incoming.Data) != entrySkillName {
		return true, false, fmt.Errorf("scaffold: invalid canonical entry bundle for %s", host)
	}
	p, hasPending := r.state.Pending[managedMapKey(r.state.Pending, target)]
	if hasPending && !r.opts.Force {
		if p.Stage == "accepted-cleanup" {
			if err := r.finishEntryMigration(target, p); err != nil {
				return r.blockEntry(target, err)
			}
			r.result.Migrated = append(r.result.Migrated, target)
			if err := r.updateFile(incoming); err != nil {
				return true, false, err
			}
			ready, err := r.entryReady(target)
			return true, ready, err
		}
		if p.IncomingSHA256 != contentDigest(incoming.Data) {
			return r.blockEntry(target, entryProblem("pending vendor changed; candidate retained at %s", p.Candidate))
		}
		r.reportPendingConflict(target, p)
		return true, false, nil
	}
	sources, err := r.collectEntrySources(host, p)
	if err != nil {
		return r.blockEntry(target, err)
	}
	if len(sources) == 0 {
		return false, false, nil
	}
	local, exists, err := readInstalledFile(r.opts.Root, target)
	if err != nil {
		return true, false, err
	}
	if !r.opts.Force && exists && !bytes.Equal(local, incoming.Data) {
		return r.blockEntry(target, entryProblem("canonical target already contains custom content; both sides retained"))
	}
	var customized *entrySource
	for i := range sources {
		if sources[i].pristine {
			continue
		}
		if customized != nil && !r.opts.Force {
			return r.blockEntry(target, entryProblem("multiple customized alias sources; no automatic winner"))
		}
		customized = &sources[i]
	}
	snapshots := make([]pendingSource, 0, len(sources))
	for _, source := range sources {
		snapshots = append(snapshots, source.snapshot)
	}
	merged := incoming.Data
	input := local
	if customized != nil && !r.opts.Force {
		lines, conflicts := merge3(splitLines(canonicalEntryBytes(customized.base)), splitLines(incoming.Data), splitLines(canonicalEntryBytes(customized.local)))
		merged = []byte(strings.Join(lines, "\n"))
		input = customized.local
		if conflicts > 0 {
			if err := r.recordConflict(incoming, input, merged, snapshots); err != nil {
				return true, false, err
			}
			return true, false, nil
		}
	}
	if skillName(merged) != entrySkillName || hasToolConflictMarkers(merged) {
		return r.blockEntry(target, entryProblem("merged entry does not have canonical name; sources retained"))
	}
	if err := r.startEntryMigration(incoming, merged, input, snapshots, false); err != nil {
		return r.blockEntry(target, err)
	}
	r.result.Migrated = append(r.result.Migrated, target)
	return true, true, nil
}

func (r *updateRun) startEntryMigration(incoming managedFile, local, input []byte, sources []pendingSource, keepLocal bool) (err error) {
	var completed []string
	stage := "migration local"
	defer func() { r.recordPartial(incoming.Rel, completed, stage, err) }()
	for _, source := range sources {
		if err := r.preserve(source.Rel); err != nil {
			return err
		}
	}
	if err := r.preserve(incoming.Rel); err != nil {
		return err
	}
	full, err := projectFilePath(r.opts.Root, incoming.Rel)
	if err != nil {
		return err
	}
	baseFull, err := projectFilePath(r.opts.Root, ".mulix/.installed/"+incoming.Rel)
	if err != nil {
		return err
	}
	if !keepLocal {
		if err := r.write(full, local, incoming.Mode()); err != nil {
			return fmt.Errorf("scaffold: writing migrated entry %s: %w", incoming.Rel, err)
		}
		completed = append(completed, "local written")
	}
	stage = "migration baseline"
	if err := r.write(baseFull, incoming.Data, 0o644); err != nil {
		return fmt.Errorf("scaffold: writing migrated baseline for %s: %w", incoming.Rel, err)
	}
	completed = append(completed, "vendor baseline written")
	stage = "migration checkpoint"
	checkpoint := pendingUpdate{Stage: "accepted-cleanup", IncomingSHA256: contentDigest(incoming.Data),
		LocalSHA256: contentDigest(input), AcceptedSHA256: contentDigest(local), BackupDir: r.backups.dir, Sources: sources}
	oldRecord, recorded := r.state.Files[managedMapKey(r.state.Files, incoming.Rel)]
	oldPending, pending := r.state.Pending[managedMapKey(r.state.Pending, incoming.Rel)]
	r.state.Files[managedMapKey(r.state.Files, incoming.Rel)] = vendorRecord{SHA256: checkpoint.IncomingSHA256}
	r.state.Pending[managedMapKey(r.state.Pending, incoming.Rel)] = checkpoint
	if err := r.saveState(r.state); err != nil {
		if recorded {
			r.state.Files[managedMapKey(r.state.Files, incoming.Rel)] = oldRecord
		} else {
			delete(r.state.Files, managedMapKey(r.state.Files, incoming.Rel))
		}
		if pending {
			r.state.Pending[managedMapKey(r.state.Pending, incoming.Rel)] = oldPending
		} else {
			delete(r.state.Pending, managedMapKey(r.state.Pending, incoming.Rel))
		}
		return fmt.Errorf("scaffold: checkpoint for %s failed; old sources retained, inspect new target and backups at %s: %w", incoming.Rel, r.backups.dir, err)
	}
	completed = append(completed, "checkpoint saved")
	stage = "source cleanup"
	return r.finishEntryMigration(incoming.Rel, checkpoint)
}

func (r *updateRun) validateEntrySources(sources []pendingSource, allowMissing bool) error {
	for _, source := range sources {
		local, exists, err := readInstalledFile(r.opts.Root, source.Rel)
		if err != nil {
			return err
		}
		if exists && contentDigest(local) != source.LocalSHA256 {
			return entryProblem("source %s changed after snapshot; retained", source.Rel)
		}
		if !exists && !allowMissing && source.LocalSHA256 != contentDigest(nil) {
			return entryProblem("source %s is missing before acceptance", source.Rel)
		}
		base, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+source.Rel)
		if err != nil {
			return err
		}
		if hasBase && (source.BaselineSHA256 == "" || contentDigest(base) != source.BaselineSHA256) {
			return entryProblem("source baseline for %s changed; retained", source.Rel)
		}
		if !hasBase && !allowMissing && source.BaselineSHA256 != "" {
			return entryProblem("source baseline for %s is missing before acceptance", source.Rel)
		}
	}
	return nil
}

func (r *updateRun) validateEntryCheckpoint(target string, p pendingUpdate) error {
	local, exists, err := readInstalledFile(r.opts.Root, target)
	if err != nil {
		return err
	}
	if !exists || contentDigest(local) != p.AcceptedSHA256 || skillName(local) != entrySkillName {
		return entryProblem("accepted target %s changed or is missing; cleanup retained", target)
	}
	base, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+target)
	if err != nil {
		return err
	}
	if !hasBase || contentDigest(base) != p.IncomingSHA256 || !r.state.trustedVendor(target, base, nil) {
		return entryProblem("accepted vendor baseline for %s changed; cleanup retained", target)
	}
	if err := r.validateEntrySources(p.Sources, true); err != nil {
		return err
	}
	probe, err := projectFilePath(r.opts.Root, p.BackupDir+"/.probe")
	if err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Dir(probe))
	if os.IsNotExist(err) {
		return entryProblem("migration backup %s is missing", p.BackupDir)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return entryProblem("migration backup %s is not a directory", p.BackupDir)
	}
	for _, source := range p.Sources {
		data, backed, err := readInstalledFile(r.opts.Root, p.BackupDir+"/local/"+source.Rel)
		if err != nil {
			return err
		}
		_, exists, err := readInstalledFile(r.opts.Root, source.Rel)
		if err != nil {
			return err
		}
		if (exists || source.LocalSHA256 != contentDigest(nil)) && (!backed || contentDigest(data) != source.LocalSHA256) {
			return entryProblem("source backup for %s changed or is missing", source.Rel)
		}
		if source.BaselineSHA256 != "" {
			data, backed, err := readInstalledFile(r.opts.Root, p.BackupDir+"/baseline/"+source.Rel)
			if err != nil {
				return err
			}
			if !backed || contentDigest(data) != source.BaselineSHA256 {
				return entryProblem("baseline backup for %s changed or is missing", source.Rel)
			}
		}
	}
	return nil
}

// The accepted-cleanup record outlives every source deletion. A retry validates
// the accepted pair, backups and all remaining snapshots before deleting more.
func (r *updateRun) finishEntryMigration(target string, p pendingUpdate) (err error) {
	var completed []string
	stage := "source cleanup"
	defer func() { r.recordPartial(target, completed, stage, err) }()
	if err := r.validateEntryCheckpoint(target, p); err != nil {
		return err
	}
	for _, source := range p.Sources {
		for _, rel := range []string{source.Rel, ".mulix/.installed/" + source.Rel} {
			full, err := projectFilePath(r.opts.Root, rel)
			if err != nil {
				return err
			}
			if _, exists, err := readInstalledFile(r.opts.Root, rel); err != nil {
				return err
			} else if !exists {
				continue
			}
			if err := r.remove(full); err != nil {
				return fmt.Errorf("scaffold: cleaning source %s: %w", rel, err)
			}
			completed = append(completed, "source removed: "+rel)
			removeEmptyParents(r.opts.Root, filepath.Dir(full))
		}
	}
	stage = "source cleanup metadata"
	removed := map[string]vendorRecord{}
	for _, source := range p.Sources {
		if record, ok := r.state.Files[managedMapKey(r.state.Files, source.Rel)]; ok {
			removed[source.Rel] = record
		}
		delete(r.state.Files, managedMapKey(r.state.Files, source.Rel))
	}
	delete(r.state.Pending, managedMapKey(r.state.Pending, target))
	if err := r.saveState(r.state); err != nil {
		for rel, record := range removed {
			r.state.Files[managedMapKey(r.state.Files, rel)] = record
		}
		r.state.Pending[managedMapKey(r.state.Pending, target)] = p
		return fmt.Errorf("scaffold: saving source cleanup for %s: %w", target, err)
	}
	return nil
}

func (r *updateRun) validateAcceptance(rel string, incoming managedFile) error {
	p, ok := r.state.Pending[managedMapKey(r.state.Pending, rel)]
	if ok && p.Stage == "accepted-cleanup" {
		return r.validateEntryCheckpoint(rel, p)
	}
	p, local, err := r.acceptanceContent(rel, incoming)
	if err != nil {
		return err
	}
	if len(p.Sources) > 0 {
		if skillName(local) != entrySkillName {
			return entryProblem("resolved target %s must have name: mulix-using", rel)
		}
		return r.validateEntrySources(p.Sources, false)
	}
	return nil
}
