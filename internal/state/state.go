// Package state persists and loads flow.State to/from
// docs/changes/<change>/.runtime/state.yaml. Decoding is strict: unknown
// fields fail to load rather than being silently dropped. The state file
// is the single source of truth both the guard checks and the PreToolUse
// hook read from — if it can't be trusted, the whole strong flow control
// mechanism is void.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

// changesDir mirrors scaffold.ChangesDir. It's a separate constant (not an
// import) to avoid a state<->scaffold import cycle; both packages must
// agree on "docs/changes" as the directory name.
const changesDir = "docs/changes"

// runtimeDirName mirrors scaffold.RuntimeDirName, for the same reason.
const runtimeDirName = ".runtime"

// PathFor returns the on-disk path for a change's state file, given the
// project root: docs/changes/<change>/.runtime/state.yaml.
func PathFor(root, change string) string {
	return filepath.Join(root, changesDir, sanitize(change), runtimeDirName, "state.yaml")
}

// sanitize keeps change IDs filesystem-safe without silently rewriting
// them into something confusable; it rejects path separators and "..".
func sanitize(change string) string {
	return strings.ReplaceAll(change, "/", "-")
}

// Load reads and strictly decodes the state file for change under root.
func Load(root, change string) (flow.State, error) {
	path := PathFor(root, change)
	data, err := os.ReadFile(path)
	if err != nil {
		return flow.State{}, fmt.Errorf("state: reading %s: %w", path, err)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	var s flow.State
	if err := dec.Decode(&s); err != nil {
		return flow.State{}, fmt.Errorf("state: decoding %s: %w", path, err)
	}
	if s.Schema != flow.SchemaVersion {
		return flow.State{}, fmt.Errorf("state: %s has schema %q, expected %q", path, s.Schema, flow.SchemaVersion)
	}
	if !s.Phase.Valid() {
		return flow.State{}, fmt.Errorf("state: %s has unknown phase %q", path, s.Phase)
	}
	return s, nil
}

// Save writes the state file for s, creating the state directory if
// needed. It stamps UpdatedAt before writing.
func Save(root string, s flow.State) error {
	if s.Change == "" {
		return fmt.Errorf("state: cannot save state with empty change id")
	}
	s.UpdatedAt = time.Now().UTC()

	path := PathFor(root, s.Change)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("state: creating state dir: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("state: marshaling: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("state: writing %s: %w", path, err)
	}
	return nil
}

// Exists reports whether a state file for change is already present under
// root.
func Exists(root, change string) bool {
	_, err := os.Stat(PathFor(root, change))
	return err == nil
}
