package scaffold

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type fileAction string

const (
	actionCurrent   fileAction = "current"
	actionInstall   fileAction = "install"
	actionForce     fileAction = "force"
	actionBlocked   fileAction = "blocked"
	actionUnchanged fileAction = "unchanged"
	actionReplace   fileAction = "replace"
	actionMerge     fileAction = "merge"
)

func chooseFileAction(exists, trusted, force bool, local, base, incoming []byte) fileAction {
	if exists && bytes.Equal(local, incoming) {
		return actionCurrent
	}
	if !exists {
		return actionInstall
	}
	if force {
		return actionForce
	}
	if !trusted {
		return actionBlocked
	}
	if bytes.Equal(base, incoming) {
		return actionUnchanged
	}
	if bytes.Equal(local, base) {
		return actionReplace
	}
	return actionMerge
}

func (r *updateRun) updateFile(f managedFile) error {
	if !validManagedRel(f.Rel) {
		return fmt.Errorf("scaffold: cannot update unmanaged path %q", f.Rel)
	}
	// nil denotes an obsolete version in trustedVendor, not an empty shipped file.
	if f.Data == nil {
		f.Data = []byte{}
	}
	if handled, err := r.handlePending(f); handled || err != nil {
		return err
	}
	local, exists, err := readInstalledFile(r.opts.Root, f.Rel)
	if err != nil {
		return err
	}
	base, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+f.Rel)
	if err != nil {
		return err
	}
	trusted := hasBase && r.state.trustedVendor(f.Rel, base, f.Data)
	action := chooseFileAction(exists, trusted, r.opts.Force, local, base, f.Data)
	switch action {
	case actionBlocked:
		r.result.Blocked = append(r.result.Blocked, f.Rel+" (no trusted baseline; merge manually or use --force)")
		return nil
	case actionCurrent, actionUnchanged:
		if err := r.commitVendor(f.Rel, local, f.Data, f.Mode()); err != nil {
			return err
		}
		label := "already current"
		if action == actionUnchanged {
			label = "vendor unchanged; local edits retained"
		}
		if f.Mode() != 0o644 {
			if err := os.Chmod(filepath.Join(r.opts.Root, filepath.FromSlash(f.Rel)), f.Mode()); err != nil {
				return fmt.Errorf("scaffold: setting script permissions for %s: %w", f.Rel, err)
			}
		}
		r.result.Skipped = append(r.result.Skipped, f.Rel+" ("+label+")")
	case actionInstall, actionForce, actionReplace:
		if err := r.commitVendor(f.Rel, f.Data, f.Data, f.Mode()); err != nil {
			return err
		}
		if action == actionInstall {
			r.result.Written = append(r.result.Written, f.Rel)
		} else {
			label := f.Rel
			if action == actionForce {
				label += " (forced)"
			}
			r.result.Updated = append(r.result.Updated, label)
		}
	case actionMerge:
		merged, conflicts := merge3(splitLines(base), splitLines(f.Data), splitLines(local))
		if conflicts > 0 {
			return r.recordConflict(f, local, []byte(strings.Join(merged, "\n")), nil)
		}
		if err := r.commitVendor(f.Rel, []byte(strings.Join(merged, "\n")), f.Data, f.Mode()); err != nil {
			return err
		}
		r.result.Merged = append(r.result.Merged, f.Rel)
	}
	return nil
}

// commitVendor preserves the old pair before changing it, and records only N.
// An error between L/B/metadata writes is reported; this is not a transaction.
func (r *updateRun) commitVendor(rel string, local, incoming []byte, mode fs.FileMode) (err error) {
	var completed []string
	stage := "local write"
	defer func() { r.recordPartial(rel, completed, stage, err) }()
	if !validManagedRel(rel) {
		return fmt.Errorf("scaffold: cannot commit unmanaged path %q", rel)
	}
	oldLocal, exists, err := readInstalledFile(r.opts.Root, rel)
	if err != nil {
		return err
	}
	oldBase, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+rel)
	if err != nil {
		return err
	}
	if (exists && !bytes.Equal(oldLocal, local)) || (hasBase && !bytes.Equal(oldBase, incoming)) {
		if err := r.preserve(rel); err != nil {
			return err
		}
	}
	full, err := projectFilePath(r.opts.Root, rel)
	if err != nil {
		return err
	}
	baseFull, err := projectFilePath(r.opts.Root, ".mulix/.installed/"+rel)
	if err != nil {
		return err
	}
	if !exists || !bytes.Equal(oldLocal, local) {
		if err := r.write(full, local, mode); err != nil {
			return fmt.Errorf("scaffold: writing local %s: %w", rel, err)
		}
		completed = append(completed, "local written")
	}
	stage = "vendor baseline"
	if !hasBase || !bytes.Equal(oldBase, incoming) {
		if err := r.write(baseFull, incoming, 0o644); err != nil {
			return fmt.Errorf("scaffold: writing vendor baseline for %s: %w", rel, err)
		}
		completed = append(completed, "vendor baseline written")
	}
	stage = "vendor record"
	return r.registerVendor(rel, incoming)
}

func (r *updateRun) registerVendor(rel string, incoming []byte) error {
	record := vendorRecord{SHA256: contentDigest(incoming)}
	old, recorded := r.state.Files[managedMapKey(r.state.Files, rel)]
	if recorded && old == record {
		return nil
	}
	r.state.Files[managedMapKey(r.state.Files, rel)] = record
	if err := r.saveState(r.state); err != nil {
		if recorded {
			r.state.Files[managedMapKey(r.state.Files, rel)] = old
		} else {
			delete(r.state.Files, managedMapKey(r.state.Files, rel))
		}
		return fmt.Errorf("scaffold: saving vendor record for %s: %w", rel, err)
	}
	return nil
}
