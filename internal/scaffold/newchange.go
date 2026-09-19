// Package scaffold creates the on-disk layout for a new change (numbered
// spec and change directories) and, separately, materializes mulix's
// project-level files (.mulix/, .claude/skills, .claude/settings.json
// hooks) into a target project.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SpecsDir is where the per-change requirements doc (spec.md) lives,
// relative to the project root. It's kept apart from ChangesDir because
// a spec is meant to stay useful as a reference long after the change
// that wrote it has been archived, while ChangesDir holds the working
// artifacts produced while executing that spec.
const SpecsDir = "docs/specs"

// ChangesDir is where a change's process artifacts (plan.md, tasks.md,
// analyze.md, report.md) and its runtime state (.runtime/state.yaml)
// live, relative to the project root. It sits alongside SpecsDir under
// docs/ so both halves of a change's paper trail are easy to find in one
// place.
const ChangesDir = "docs/changes"

// RuntimeDirName is the subdirectory of a change directory that holds
// mulix's own runtime state for that change. Like the state file itself,
// nothing under here is meant to be hand-edited.
const RuntimeDirName = ".runtime"

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases s and collapses runs of non-alphanumeric characters
// into single hyphens, trimming leading/trailing hyphens. Empty input (or
// input that slugifies to empty) is rejected by the caller, not here.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlugChars.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// numberedDirRe matches the "NNN-" numbering prefix used for change
// directories.
var numberedDirRe = regexp.MustCompile(`^(\d+)-`)

// NextNumber scans root/changes for existing "NNN-*" directories and
// returns the next unused number, starting at 1. A missing changes
// directory yields 1. It scans ChangesDir rather than SpecsDir because a
// change directory is created for every change from the moment `mulix
// new` runs, while the spec directory only appears once spec.md is
// actually written.
func NextNumber(root string) (int, error) {
	dir := filepath.Join(root, ChangesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("scaffold: reading %s: %w", dir, err)
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := numberedDirRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}

// ChangeID builds the "NNN-slug" change identifier used as both
// directories' name and as the state file's change id.
func ChangeID(number int, slug string) string {
	return fmt.Sprintf("%03d-%s", number, slug)
}

// NewChange is the result of creating a new change's artifact
// directories.
type NewChange struct {
	Change string // e.g. "001-add-login"

	SpecDir    string // root-relative, e.g. "docs/specs/001-add-login"
	SpecAbsDir string

	ChangeDir    string // root-relative, e.g. "docs/changes/001-add-login"
	ChangeAbsDir string

	Slug   string
	Number int
}

// CreateChangeDir allocates the next number for title, slugifies it, and
// creates the (empty) docs/specs/<NNN-slug>/ and docs/changes/<NNN-slug>/
// directories under root. It does not write spec.md or any other
// artifact — that happens in the specify phase.
func CreateChangeDir(root, title string) (NewChange, error) {
	slug := Slugify(title)
	if slug == "" {
		return NewChange{}, fmt.Errorf("scaffold: %q has no usable slug characters", title)
	}
	number, err := NextNumber(root)
	if err != nil {
		return NewChange{}, err
	}
	change := ChangeID(number, slug)

	specRelDir := filepath.Join(SpecsDir, change)
	specAbsDir := filepath.Join(root, specRelDir)
	if err := os.MkdirAll(specAbsDir, 0o755); err != nil {
		return NewChange{}, fmt.Errorf("scaffold: creating %s: %w", specRelDir, err)
	}

	changeRelDir := filepath.Join(ChangesDir, change)
	changeAbsDir := filepath.Join(root, changeRelDir)
	if err := os.MkdirAll(changeAbsDir, 0o755); err != nil {
		return NewChange{}, fmt.Errorf("scaffold: creating %s: %w", changeRelDir, err)
	}

	return NewChange{
		Change:       change,
		SpecDir:      filepath.ToSlash(specRelDir),
		SpecAbsDir:   specAbsDir,
		ChangeDir:    filepath.ToSlash(changeRelDir),
		ChangeAbsDir: changeAbsDir,
		Slug:         slug,
		Number:       number,
	}, nil
}
