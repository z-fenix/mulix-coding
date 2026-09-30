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

	"github.com/z-fenix/mulix-coding/internal/layout"
)

// SpecsDir and ChangesDir re-export internal/layout's names so existing
// scaffold callers keep one import.
const (
	SpecsDir   = layout.SpecsDir
	ChangesDir = layout.ChangesDir
)

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

	RuntimeDir string // root-relative, e.g. ".mulix/.runtime/001-add-login"

	Slug   string
	Number int
}

// CreateChangeDir allocates the next number for title, slugifies it, and
// creates the (empty) docs/specs/<NNN-slug>/, docs/changes/<NNN-slug>/,
// and .mulix/.runtime/<NNN-slug>/ directories under root. It does not
// write spec.md or any other artifact — that happens in the specify
// phase.
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

	runtimeAbsDir := filepath.Join(root, filepath.FromSlash(layout.ChangeRuntimeDir(change)))
	if err := os.MkdirAll(runtimeAbsDir, 0o755); err != nil {
		return NewChange{}, fmt.Errorf("scaffold: creating %s: %w", layout.ChangeRuntimeDir(change), err)
	}

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
		RuntimeDir:   layout.ChangeRuntimeDir(change),
		Slug:         slug,
		Number:       number,
	}, nil
}
