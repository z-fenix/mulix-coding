// Package state persists and loads flow.State to/from
// .mulix/.runtime/<change>/state.yaml. Decoding is strict: unknown fields
// fail to load rather than being silently dropped. The state file is the
// single source of truth both the guard checks and the PreToolUse hook
// read from. These checks rely on recorded evidence, not authenticated
// approval or proof of test execution; native artifact validation errors are
// distinguished so hook callers can fail closed.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
)

// PathFor returns the on-disk path for a change's state file, given the
// project root: .mulix/.runtime/<change>/state.yaml.
func PathFor(root, change string) string {
	return filepath.Join(root, filepath.FromSlash(layout.ChangeRuntimePath(sanitize(change), layout.StateFile)))
}

// legacyPathFor is where mulix.state.v1 kept a change's state file. Load
// only uses it to explain a failure, never to read state from.
func legacyPathFor(root, change string) string {
	return filepath.Join(root, filepath.FromSlash(layout.ChangesDir), sanitize(change), ".runtime", "state.yaml")
}

// sanitize keeps change IDs filesystem-safe without silently rewriting
// them into something confusable; it rejects path separators and "..".
func sanitize(change string) string {
	return strings.ReplaceAll(change, "/", "-")
}

// ErrInvalidArtifacts identifies invalid native artifact metadata for callers
// that must distinguish it from legacy state errors.
var ErrInvalidArtifacts = errors.New("invalid native artifact metadata")

func nativeArtifactFields(data []byte) bool {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil {
		// Emitted top-level native keys still identify native state when a
		// later syntax error prevents the YAML decoder from building a tree.
		for _, line := range strings.Split(string(data), "\n") {
			for _, key := range []string{"artifact_layout:", "tasks_dir:", "tracker:"} {
				if line == key || strings.HasPrefix(line, key+" ") || strings.HasPrefix(line, key+"\t") || line == key+"\r" {
					return true
				}
			}
		}
		return false
	}
	if len(doc.Content) == 0 {
		return false
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		switch mapping.Content[i].Value {
		case "artifact_layout", "tasks_dir", "tracker":
			return true
		}
	}
	return false
}

// nativeFootprint checks for an ordinary runtime issues directory beside the
// state file. It is only consulted when the state cannot be read or decoded.
func nativeFootprint(root, change string) bool {
	dir := filepath.Join(filepath.Dir(PathFor(root, change)), layout.RuntimeIssuesDir)
	info, err := os.Lstat(dir)
	return err == nil && info.Mode().IsDir()
}

// Load reads and strictly decodes the state file for change under root.
func Load(root, change string) (flow.State, error) {
	path := PathFor(root, change)
	data, err := os.ReadFile(path)
	if err != nil {
		if nativeFootprint(root, change) {
			return flow.State{}, fmt.Errorf("%w: reading %s: %w", ErrInvalidArtifacts, path, err)
		}
		if _, legacyErr := os.Stat(legacyPathFor(root, change)); legacyErr == nil {
			return flow.State{}, fmt.Errorf("state: %s not found, but a mulix.state.v1 file exists at %s; this mulix keeps state under %s with a different phase model — start the change again with `mulix new`", path, legacyPathFor(root, change), layout.RuntimeDir)
		}
		return flow.State{}, fmt.Errorf("state: reading %s: %w", path, err)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	var s flow.State
	if err := dec.Decode(&s); err != nil {
		if nativeArtifactFields(data) || nativeFootprint(root, change) {
			return flow.State{}, fmt.Errorf("%w: decoding %s: %w", ErrInvalidArtifacts, path, err)
		}
		return flow.State{}, fmt.Errorf("state: decoding %s: %w", path, err)
	}
	if s.Schema != flow.SchemaVersion {
		err := fmt.Errorf("state: %s has schema %q, expected %q", path, s.Schema, flow.SchemaVersion)
		if nativeArtifactFields(data) {
			return flow.State{}, fmt.Errorf("%w: %w", ErrInvalidArtifacts, err)
		}
		return flow.State{}, err
	}
	if !s.Phase.Valid() {
		err := fmt.Errorf("state: %s has unknown phase %q", path, s.Phase)
		if nativeArtifactFields(data) {
			return flow.State{}, fmt.Errorf("%w: %w", ErrInvalidArtifacts, err)
		}
		return flow.State{}, err
	}
	if err := validateArtifacts(root, s); err != nil {
		return flow.State{}, fmt.Errorf("%w: %w", ErrInvalidArtifacts, err)
	}
	if s.ArtifactLayout != "" && s.Change != change {
		return flow.State{}, fmt.Errorf("%w: state: pinned change identity %q does not match %q", ErrInvalidArtifacts, s.Change, change)
	}
	return s, nil
}

// validateArtifacts checks new fields without imposing new rules on untouched legacy records.
func validateArtifacts(root string, s flow.State) error {
	switch s.ArtifactLayout {
	case "":
		if s.Tracker != nil || s.TasksDir != "" {
			return fmt.Errorf("state: tracker and tasks_dir require artifact_layout %q", flow.RuntimeIssuesLayout)
		}
		return nil
	case flow.RuntimeIssuesLayout:
	default:
		return fmt.Errorf("state: unknown artifact_layout %q; use a compatible mulix version", s.ArtifactLayout)
	}
	if s.Change == "" || s.Change == layout.Shared || s.Change == "." || s.Change == ".." || strings.ContainsAny(s.Change, "/\\:") {
		return fmt.Errorf("state: invalid pinned change id %q", s.Change)
	}
	if s.Tracker == nil {
		return fmt.Errorf("state: runtime-issues-v1 requires a pinned tracker")
	}
	if err := s.Tracker.Validate(); err != nil {
		return fmt.Errorf("state: invalid pinned tracker: %w", err)
	}
	if s.SpecPath != layout.ChangeRuntimePath(s.Change, layout.RuntimeSpecFile) ||
		s.TasksDir != layout.ChangeRuntimePath(s.Change, layout.RuntimeIssuesDir) ||
		s.ReportPath != layout.ChangeRuntimePath(s.Change, layout.RuntimeReportFile) || s.TasksPath != "" {
		return fmt.Errorf("state: runtime-issues-v1 paths must match the owning change's standard runtime paths")
	}
	for slot, m := range s.Tracker.Issues {
		if slot == 0 && m.Path == s.SpecPath {
			continue
		}
		name, owned := strings.CutPrefix(m.Path, s.TasksDir+"/")
		digits, _, hasDash := strings.Cut(name, "-")
		n, err := strconv.Atoi(digits)
		if slot <= 0 || !owned || strings.ContainsAny(name, "/\\:\x00") || !strings.HasSuffix(name, ".md") ||
			!hasDash || len(digits) < 2 || strings.Trim(digits, "0123456789") != "" || err != nil || n != slot {
			return fmt.Errorf("state: Issue slot %d does not use its owning change's standard path", slot)
		}
	}
	for _, p := range []string{s.SpecPath, s.TasksDir + "/.probe", s.ReportPath, layout.ChangeRuntimePath(s.Change, layout.StateFile)} {
		if _, err := fileutil.ControlPath(root, p, false); err != nil {
			return fmt.Errorf("state: unsafe runtime artifact %s: %w", p, err)
		}
	}
	return nil
}

// Save writes the state file for s, creating the state directory if
// needed. It stamps UpdatedAt before writing.
func Save(root string, s flow.State) error {
	return save(root, s, fileutil.WriteFile)
}

// SaveTrackerCheckpoint persists a publication intent, positive receipt or
// binding before the caller continues. It retains all normal validation and
// is only for a locked, native remote change, not a legacy state write.
func SaveTrackerCheckpoint(root string, s flow.State) error {
	if s.ArtifactLayout != flow.RuntimeIssuesLayout || s.Tracker == nil || s.Tracker.Target.Kind == "markdown" || len(s.Tracker.Issues) == 0 {
		return fmt.Errorf("tracker checkpoint requires a native remote Issue publication")
	}
	return save(root, s, func(path string, data []byte, mode os.FileMode) error {
		return fileutil.WriteFileDurable(root, path, data, mode)
	})
}

func save(root string, s flow.State, write func(string, []byte, os.FileMode) error) error {
	if s.Change == "" {
		return fmt.Errorf("state: cannot save state with empty change id")
	}
	if s.Change == layout.Shared {
		return fmt.Errorf("state: %q is reserved and cannot be a change id", layout.Shared)
	}
	if err := validateArtifacts(root, s); err != nil {
		return err
	}
	// Once a binding is recorded, no ordinary save can redirect or downgrade it.
	// Legacy-to-runtime conversion is deliberately not a migration API either.
	path := PathFor(root, s.Change)
	if data, err := os.ReadFile(path); err == nil {
		var existing struct {
			ArtifactLayout string `yaml:"artifact_layout"`
		}
		decodeErr := yaml.Unmarshal(data, &existing)
		if s.ArtifactLayout != "" || existing.ArtifactLayout != "" {
			if decodeErr != nil {
				return fmt.Errorf("state: reading existing pinned state: %w", decodeErr)
			}
			previous, err := Load(root, s.Change)
			if err != nil {
				return err
			}
			if previous.ArtifactLayout != s.ArtifactLayout || previous.Tracker == nil || s.Tracker == nil {
				return fmt.Errorf("state: artifact layout and tracker binding are immutable; create a new change")
			}
			oldTarget, newTarget := previous.Tracker.Target, s.Tracker.Target
			if oldTarget.Kind != newTarget.Kind || oldTarget.Host != newTarget.Host || oldTarget.Project != newTarget.Project || previous.Tracker.RemoteKey != s.Tracker.RemoteKey || (oldTarget.ProjectID != "" && oldTarget.ProjectID != newTarget.ProjectID) {
				return fmt.Errorf("state: tracker selector, binding key, and verified project ID are immutable; create a new change")
			}
			for slot, old := range previous.Tracker.Issues {
				next, ok := s.Tracker.Issues[slot]
				changedReceipt := old.Receipt != nil && (next.Receipt == nil || *old.Receipt != *next.Receipt)
				if !ok || next.Path != old.Path || next.DraftDigest != old.DraftDigest || next.PublishDigest != old.PublishDigest ||
					changedReceipt || (old.Published() && (next.ID != old.ID || next.Number != old.Number || next.NodeID != old.NodeID)) {
					return fmt.Errorf("state: Issue slot %d cannot change its path, publication intent, positive create receipt or established identity", slot)
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("state: reading existing state: %w", err)
	}
	s.UpdatedAt = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("state: creating state dir: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("state: marshaling: %w", err)
	}
	if err := writeStateFileWith(path, data, write); err != nil {
		return fmt.Errorf("state: writing %s: %w", path, err)
	}
	return nil
}

func writeStateFile(path string, data []byte) error {
	return writeStateFileWith(path, data, fileutil.WriteFile)
}

// writeStateFileWith preserves an existing file's permissions while preparing
// the complete replacement; it does not provide concurrent-writer isolation.
func writeStateFileWith(path string, data []byte, write func(string, []byte, os.FileMode) error) error {
	mode := os.FileMode(0o644)
	if info, err := os.Lstat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return write(path, data, mode)
}

// Exists reports whether a state file for change is already present under
// root.
func Exists(root, change string) bool {
	_, err := os.Stat(PathFor(root, change))
	return err == nil
}
