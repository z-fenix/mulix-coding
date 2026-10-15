// Package scaffold materializes project resources and allocates date-named
// specification, change and runtime directories without migrating old IDs.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/z-fenix/mulix-coding/internal/layout"
)

const (
	SpecsDir   = layout.SpecsDir
	ChangesDir = layout.ChangesDir
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify keeps the existing lower-case ASCII slug policy.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlugChars.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

type NewChange struct {
	Change       string // e.g. 2026-10-14-add-login
	SpecDir      string // root-relative
	SpecAbsDir   string
	ChangeDir    string // root-relative
	ChangeAbsDir string
	RuntimeDir   string // root-relative
	Slug         string
}

// CreateChangeDir captures the local date once. Artifact filenames remain
// spec.md/tasks.md/report.md; their phase skills create them later.
func CreateChangeDir(root, title string) (NewChange, error) {
	return createChangeDirAt(root, title, time.Now())
}

func changeCandidate(date, slug string, attempt int) string {
	base := date + "-" + slug
	if attempt == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, attempt)
}

func createChangeDirAt(root, title string, now time.Time) (NewChange, error) {
	slug := Slugify(title)
	if slug == "" {
		return NewChange{}, fmt.Errorf("scaffold: %q has no usable slug characters", title)
	}
	date := now.Format("2006-01-02")
	for _, parent := range []string{layout.RuntimeDir, SpecsDir, ChangesDir} {
		probe, err := projectFilePath(root, parent+"/.probe")
		if err != nil {
			return NewChange{}, err
		}
		if err := os.MkdirAll(filepath.Dir(probe), 0o755); err != nil {
			return NewChange{}, fmt.Errorf("scaffold: creating %s: %w", parent, err)
		}
	}
	for attempt := 1; ; attempt++ {
		change := changeCandidate(date, slug, attempt)
		nc := NewChange{Change: change, Slug: slug,
			SpecDir:    filepath.ToSlash(filepath.Join(SpecsDir, change)),
			ChangeDir:  filepath.ToSlash(filepath.Join(ChangesDir, change)),
			RuntimeDir: layout.ChangeRuntimeDir(change)}
		nc.SpecAbsDir = filepath.Join(root, filepath.FromSlash(nc.SpecDir))
		nc.ChangeAbsDir = filepath.Join(root, filepath.FromSlash(nc.ChangeDir))
		runtime := filepath.Join(root, filepath.FromSlash(nc.RuntimeDir))
		dirs := []string{runtime, nc.SpecAbsDir, nc.ChangeAbsDir}
		collision := false
		for _, dir := range dirs {
			if _, err := os.Lstat(dir); err == nil {
				collision = true
				break
			} else if !os.IsNotExist(err) {
				return NewChange{}, fmt.Errorf("scaffold: inspecting candidate %s: %w", dir, err)
			}
		}
		if collision {
			continue
		}
		// Mkdir reserves this ID. This is not a lock or cross-directory transaction.
		if err := os.Mkdir(runtime, 0o755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return NewChange{}, fmt.Errorf("scaffold: reserving %s: %w", nc.RuntimeDir, err)
		}
		created := []string{runtime}
		retry := false
		for _, dir := range dirs[1:] {
			if err := os.Mkdir(dir, 0o755); err != nil {
				// Remove only this attempt's new empty directories, never recursively.
				for i := len(created) - 1; i >= 0; i-- {
					_ = os.Remove(created[i])
				}
				if os.IsExist(err) {
					retry = true
					break
				}
				return NewChange{}, fmt.Errorf("scaffold: creating %s: %w", dir, err)
			}
			created = append(created, dir)
		}
		if retry {
			continue
		}
		return nc, nil
	}
}
