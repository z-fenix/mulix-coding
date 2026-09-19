package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Dir is the directory (relative to a project root) where installed
// presets and the registry live.
const Dir = ".mulix/presets"

// registryFile is the registry's filename, matching spec-kit's
// .specify/presets/.registry (a JSON file despite the extension-less name).
const registryFile = ".registry"

// DefaultPriority is used when a preset is installed without an explicit
// priority. Lower numbers take precedence, matching spec-kit's convention.
const DefaultPriority = 10

// Entry is one installed preset's registry record.
type Entry struct {
	Version      string    `json:"version"`
	Source       string    `json:"source"` // local dir path or URL the preset was installed from
	ManifestHash string    `json:"manifest_hash"`
	Enabled      bool      `json:"enabled"`
	Priority     int       `json:"priority"`
	InstalledAt  time.Time `json:"installed_at"`
}

// Registry is the full set of installed presets for one project, keyed by
// preset id.
type Registry struct {
	Presets map[string]Entry `json:"presets"`
}

func registryPath(root string) string {
	return filepath.Join(root, Dir, registryFile)
}

// LoadRegistry reads the registry for root. A missing file is not an
// error: it is treated as an empty registry, since a project with no
// presets installed yet simply has none.
func LoadRegistry(root string) (Registry, error) {
	path := registryPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Presets: map[string]Entry{}}, nil
		}
		return Registry{}, fmt.Errorf("preset: reading %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, fmt.Errorf("preset: decoding %s: %w", path, err)
	}
	if reg.Presets == nil {
		reg.Presets = map[string]Entry{}
	}
	return reg, nil
}

// Save writes reg to root's registry file, creating the presets directory
// if needed.
func Save(root string, reg Registry) error {
	path := registryPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("preset: creating %s: %w", filepath.Dir(path), err)
	}
	if reg.Presets == nil {
		reg.Presets = map[string]Entry{}
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("preset: encoding registry: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("preset: writing %s: %w", path, err)
	}
	return nil
}

// Add records id in the registry for root, overwriting any existing entry
// with the same id.
func Add(root, id string, entry Entry) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	reg.Presets[id] = entry
	return Save(root, reg)
}

// Remove deletes id's registry entry. It does not remove the preset's
// files on disk; callers that also want that should do it separately, so
// a failed disk removal can't leave the registry claiming a preset that
// no longer has files, or vice versa, without the caller knowing.
func Remove(root, id string) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	if _, ok := reg.Presets[id]; !ok {
		return fmt.Errorf("preset: %q is not installed", id)
	}
	delete(reg.Presets, id)
	return Save(root, reg)
}

// SetEnabled flips id's enabled flag.
func SetEnabled(root, id string, enabled bool) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	entry, ok := reg.Presets[id]
	if !ok {
		return fmt.Errorf("preset: %q is not installed", id)
	}
	entry.Enabled = enabled
	reg.Presets[id] = entry
	return Save(root, reg)
}

// SetPriority sets id's priority (lower = higher precedence).
func SetPriority(root, id string, priority int) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	entry, ok := reg.Presets[id]
	if !ok {
		return fmt.Errorf("preset: %q is not installed", id)
	}
	entry.Priority = priority
	reg.Presets[id] = entry
	return Save(root, reg)
}

// ByPriority is one registry entry paired with its id, as returned by
// ListByPriority.
type ByPriority struct {
	ID    string
	Entry Entry
}

// ListByPriority returns every enabled preset (or every preset, if
// includeDisabled is true), sorted by priority ascending and then by id,
// matching spec-kit's list_by_priority ordering.
func ListByPriority(reg Registry, includeDisabled bool) []ByPriority {
	var out []ByPriority
	for id, entry := range reg.Presets {
		if !entry.Enabled && !includeDisabled {
			continue
		}
		out = append(out, ByPriority{ID: id, Entry: entry})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entry.Priority != out[j].Entry.Priority {
			return out[i].Entry.Priority < out[j].Entry.Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// InstallDir returns the on-disk directory a preset with id is (or would
// be) installed into under root.
func InstallDir(root, id string) string {
	return filepath.Join(root, Dir, id)
}
