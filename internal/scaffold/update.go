package scaffold

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mulix-dev/mulix-coding/assets"
)

// UpdateResult classifies what update did to each file, so the CLI can
// print an honest per-category summary.
type UpdateResult struct {
	Written    []string // file was missing on disk; fresh copy installed
	Updated    []string // untouched since install; replaced by the new version
	Merged     []string // locally modified; three-way merge applied cleanly
	Conflicted []string // locally modified; merge left Git-style conflict markers
	Skipped    []string // left alone (already current, or no baseline to merge against)
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

// Update refreshes the bundled skills (.claude/skills/*/SKILL.md) and
// templates (.mulix/templates/*) in a target project to the versions in
// this mulix binary. Files the user hasn't touched are updated in place;
// locally modified files are three-way merged against the baseline copy
// recorded at install time (.mulix/.installed/), with real conflicts
// left in the file as Git-style markers and reported. Files without a
// baseline (installed by an older mulix) are skipped and reported —
// guessing a merge base isn't safe.
//
// update does not touch .mulix/memory/constitution.md (a user-authored
// document), .claude/settings.json (init's hook merge already handles
// it), or .mulix/presets/ (deliberate overrides, not core content).
func Update(opts UpdateOptions) (UpdateResult, error) {
	var res UpdateResult

	skills, err := fs.ReadDir(assets.Skills, "skills")
	if err != nil {
		return res, err
	}
	for _, e := range skills {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(assets.Skills, "skills/"+e.Name()+"/SKILL.md")
		if err != nil {
			return res, err
		}
		rel := ".claude/skills/" + e.Name() + "/SKILL.md"
		if err := updateFile(&res, opts, rel, string(data)); err != nil {
			return res, err
		}
	}

	templates, err := fs.ReadDir(assets.Templates, "templates")
	if err != nil {
		return res, err
	}
	for _, e := range templates {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(assets.Templates, "templates/"+e.Name())
		if err != nil {
			return res, err
		}
		rel := ".mulix/templates/" + e.Name()
		if err := updateFile(&res, opts, rel, string(data)); err != nil {
			return res, err
		}
	}

	return res, nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// splitLines splits file content into merge3's line representation.
// Splitting on "\n" without trimming keeps the trailing-newline state
// intact through a merge round-trip: a file ending in "\n" yields a
// final empty element that Join restores.
func splitLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}
