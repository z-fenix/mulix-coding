package scaffold

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// UpdateResult classifies what update did to each file, so the CLI can
// print an honest per-category summary.
type UpdateResult struct {
	Written    []string // file was missing on disk; fresh copy installed
	Updated    []string // untouched since install; replaced by the new version
	Merged     []string // locally modified; three-way merge applied cleanly
	Conflicted []string // locally modified; merge left Git-style conflict markers
	Removed    []string // no longer shipped and untouched since install; deleted
	Skipped    []string // left alone (already current, no baseline to merge against, or modified obsolete file)
}

// UpdateOptions controls what update writes into a target project.
type UpdateOptions struct {
	// Root is the target project directory (must already exist).
	Root string
	// Force overwrites every managed file with the bundled version and
	// rebaselines, discarding local edits — the escape hatch when a
	// three-way merge isn't worth saving.
	Force bool
}

// Update refreshes every managed file (see managedFiles: mulix's skills,
// the embedded superpowers skills with their scripts and prompts, and the
// templates) in a target project to the versions in this mulix binary.
// Files the user hasn't touched are updated in place; locally modified
// files are three-way merged against the baseline copy recorded at
// install time (.mulix/.installed/), with real conflicts left in the file
// as Git-style markers and reported. Files without a baseline (installed
// by an older mulix) are skipped and reported — guessing a merge base
// isn't safe. Files this mulix no longer ships are removed if untouched.
//
// update does not touch .mulix/memory/constitution.md (a user-authored
// document), .claude/settings.json (init's hook merge already handles
// it), .mulix/presets/ (deliberate overrides, not core content), or
// .mulix/.runtime/ (the changes' own records).
func Update(opts UpdateOptions) (UpdateResult, error) {
	var res UpdateResult

	files, err := managedFiles()
	if err != nil {
		return res, err
	}
	for _, f := range files {
		if err := updateFile(&res, opts, f.Rel, string(f.Data)); err != nil {
			return res, err
		}
		if f.Mode() != 0o644 {
			// Keep scripts executable even after a merge rewrote them.
			if err := os.Chmod(filepath.Join(opts.Root, filepath.FromSlash(f.Rel)), f.Mode()); err != nil {
				return res, err
			}
		}
	}

	for _, rel := range obsoleteFiles {
		if err := removeObsolete(&res, opts, rel); err != nil {
			return res, err
		}
	}

	return res, nil
}

// removeObsolete deletes a file an older mulix installed and this one no
// longer ships — only when it (and its baseline) still match exactly, so
// a user's edits are never thrown away. Anything else is reported.
func removeObsolete(res *UpdateResult, opts UpdateOptions, relPath string) error {
	full := filepath.Join(opts.Root, filepath.FromSlash(relPath))
	baseFull := baselinePath(opts.Root, relPath)
	cur, err := os.ReadFile(full)
	if os.IsNotExist(err) {
		_ = os.Remove(baseFull)
		return nil
	} else if err != nil {
		return err
	}
	base, err := os.ReadFile(baseFull)
	if (err == nil && bytes.Equal(cur, base)) || opts.Force {
		if err := os.Remove(full); err != nil {
			return err
		}
		_ = os.Remove(baseFull)
		removeEmptyParents(opts.Root, filepath.Dir(full))
		removeEmptyParents(opts.Root, filepath.Dir(baseFull))
		res.Removed = append(res.Removed, relPath)
		return nil
	}
	res.Skipped = append(res.Skipped, relPath+" (no longer shipped, but locally modified; remove it by hand)")
	return nil
}

// removeEmptyParents removes dir and its empty ancestors up to (not
// including) root.
func removeEmptyParents(root, dir string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
	}
}

// updateFile brings one managed file up to the incoming (bundled)
// version, classifying the outcome into res. See Update for the
// decision table.
func updateFile(res *UpdateResult, opts UpdateOptions, relPath string, incoming string) error {
	full := filepath.Join(opts.Root, filepath.FromSlash(relPath))
	baseFull := baselinePath(opts.Root, relPath)

	cur, err := os.ReadFile(full)
	if os.IsNotExist(err) {
		if writeErr := writeBoth(full, baseFull, []byte(incoming)); writeErr != nil {
			return writeErr
		}
		res.Written = append(res.Written, relPath)
		return nil
	} else if err != nil {
		return err
	}

	if opts.Force {
		if writeErr := writeBoth(full, baseFull, []byte(incoming)); writeErr != nil {
			return writeErr
		}
		res.Updated = append(res.Updated, relPath+" (forced)")
		return nil
	}

	if bytes.Equal(cur, []byte(incoming)) {
		// Already current; make sure the baseline matches so future
		// merges see the right common ancestor.
		base, baseErr := os.ReadFile(baseFull)
		if baseErr != nil || !bytes.Equal(base, []byte(incoming)) {
			if writeErr := writeFile(baseFull, []byte(incoming)); writeErr != nil {
				return writeErr
			}
		}
		res.Skipped = append(res.Skipped, relPath+" (already current)")
		return nil
	}

	base, err := os.ReadFile(baseFull)
	if os.IsNotExist(err) {
		res.Skipped = append(res.Skipped, relPath+" (no baseline; merge manually or use --force)")
		return nil
	} else if err != nil {
		return err
	}

	if bytes.Equal(cur, base) {
		if writeErr := writeBoth(full, baseFull, []byte(incoming)); writeErr != nil {
			return writeErr
		}
		res.Updated = append(res.Updated, relPath)
		return nil
	}

	merged, conflicts := merge3(splitLines(base), splitLines([]byte(incoming)), splitLines(cur))
	out := []byte(strings.Join(merged, "\n"))
	if writeErr := writeBoth(full, baseFull, out); writeErr != nil {
		return writeErr
	}
	if conflicts > 0 {
		res.Conflicted = append(res.Conflicted, relPath)
	} else {
		res.Merged = append(res.Merged, relPath)
	}
	return nil
}

// baselinePath mirrors relPath under .mulix/.installed/ to hold the
// bytes mulix wrote at install/update time — the common ancestor for
// later three-way merges.
func baselinePath(root, relPath string) string {
	return filepath.Join(root, ".mulix", ".installed", filepath.FromSlash(relPath))
}

// writeBoth writes the file and its baseline copy in one step.
func writeBoth(full, baseFull string, data []byte) error {
	if err := writeFile(full, data); err != nil {
		return err
	}
	return writeFile(baseFull, data)
}

// writeFile writes data to path, creating parent directories as needed.
func writeFile(path string, data []byte) error {
	return writeFileMode(path, data, 0o644)
}

// writeFileMode is writeFile with explicit permission bits. os.WriteFile
// only applies the mode when creating the file, so an existing file is
// chmod-ed too.
func writeFileMode(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// splitLines splits file content into merge3's line representation.
// Splitting on "\n" without trimming keeps the trailing-newline state
// intact through a merge round-trip: a file ending in "\n" yields a
// final empty element that Join restores.
func splitLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}
