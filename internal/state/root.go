package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoRoot is returned by FindRoot when no .mulix directory is found
// walking up from the starting directory.
var ErrNoRoot = errors.New("state: no .mulix directory found (run `mulix init` first)")

// activeFile records which change is "current" for commands that don't
// take an explicit --change flag.
const activeFile = ".mulix/active"

// FindRoot walks up from start looking for a .mulix directory, the same
// way git walks up looking for .git. It returns the directory containing
// .mulix, not the .mulix directory itself.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("state: resolving start dir: %w", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ".mulix")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoRoot
		}
		dir = parent
	}
}

// SetActive records change as the current active change for root.
func SetActive(root, change string) error {
	path := filepath.Join(root, activeFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("state: creating .mulix dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(change)+"\n"), 0o644); err != nil {
		return fmt.Errorf("state: writing active change: %w", err)
	}
	return nil
}

// Active returns the current active change for root, or an error if none
// has been set yet.
func Active(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, activeFile))
	if err != nil {
		return "", fmt.Errorf("state: no active change set (use `mulix new` or `mulix state select`): %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ClearActive removes the active-change marker for root, if one is set. It
// is not an error for none to exist. Call this once a change is archived so
// commands that omit --change fail loudly (pointing the user at `mulix
// state select` or `mulix new`) instead of silently continuing to operate
// on a change that's done.
func ClearActive(root string) error {
	err := os.Remove(filepath.Join(root, activeFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("state: clearing active change: %w", err)
	}
	return nil
}

// ActiveIs reports whether change is currently the active change for root.
// It treats "no active change set" as false rather than an error, since
// callers use this to decide whether to clear the marker, not to require
// one exists.
func ActiveIs(root, change string) bool {
	active, err := Active(root)
	if err != nil {
		return false
	}
	return active == change
}

// List returns the change ids of every change directory under root that
// has a state file, sorted alphabetically. Used by `mulix status` to
// enumerate all changes rather than just the active one.
func List(root string) ([]string, error) {
	dir := filepath.Join(root, changesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("state: listing %s: %w", dir, err)
	}
	var changes []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), runtimeDirName, "state.yaml")); err != nil {
			continue
		}
		changes = append(changes, e.Name())
	}
	sort.Strings(changes)
	return changes, nil
}
