// Package preset implements mulix's preset system: a Go port of spec-kit's
// preset manifest / registry / template-override stack, scoped to what
// mulix needs (no extensions layer, so the stack is project overrides ->
// installed presets (by priority) -> core bundled, one tier shorter than
// spec-kit's four).
package preset

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestSchemaVersion is the schema_version every preset.yml must declare.
const ManifestSchemaVersion = "1.0"

// Manifest is the Go equivalent of spec-kit's preset.yml.
type Manifest struct {
	SchemaVersion string     `yaml:"schema_version"`
	Preset        PresetInfo `yaml:"preset"`
	Requires      Requires   `yaml:"requires,omitempty"`
	Provides      Provides   `yaml:"provides"`
	Tags          []string   `yaml:"tags,omitempty"`
}

// PresetInfo identifies the preset itself.
type PresetInfo struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
	Author      string `yaml:"author,omitempty"`
	Repository  string `yaml:"repository,omitempty"`
	License     string `yaml:"license,omitempty"`
}

// Requires declares compatibility constraints. mulix does not currently
// enforce MulixVersion (no version-range parser has been ported), but the
// field is kept so manifests round-trip and a future check has somewhere
// to read from.
type Requires struct {
	MulixVersion string `yaml:"mulix_version,omitempty"`
}

// Provides lists what a preset contributes.
type Provides struct {
	Templates []TemplateEntry `yaml:"templates"`
}

// Strategy is a template composition strategy, mirroring spec-kit's
// resolve_template_content strategies.
type Strategy string

const (
	StrategyReplace Strategy = "replace"
	StrategyPrepend Strategy = "prepend"
	StrategyAppend  Strategy = "append"
	StrategyWrap    Strategy = "wrap"
)

// Valid reports whether s is one of the known strategies.
func (s Strategy) Valid() bool {
	switch s {
	case StrategyReplace, StrategyPrepend, StrategyAppend, StrategyWrap:
		return true
	default:
		return false
	}
}

// TemplateEntry declares one template a preset provides or overrides.
type TemplateEntry struct {
	Type        string   `yaml:"type"` // "template" (mulix only has this one kind, unlike spec-kit's template|command|script)
	Name        string   `yaml:"name"`
	File        string   `yaml:"file"`
	Description string   `yaml:"description,omitempty"`
	Strategy    Strategy `yaml:"strategy,omitempty"` // defaults to "replace" if empty
	Replaces    string   `yaml:"replaces,omitempty"`
}

// EffectiveStrategy returns t.Strategy, defaulting to "replace" the same
// way spec-kit's manifests do.
func (t TemplateEntry) EffectiveStrategy() Strategy {
	if t.Strategy == "" {
		return StrategyReplace
	}
	return t.Strategy
}

// LoadManifest strictly decodes a preset.yml at path. Unknown fields are
// rejected, matching the state package's fail-loud discipline: a preset
// manifest is data other commands trust, not free-form notes.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("preset: reading %s: %w", path, err)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("preset: decoding %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("preset: %s: %w", path, err)
	}
	return m, nil
}

// Validate checks the structural invariants LoadManifest and the installer
// both rely on: a schema version, a non-empty preset id, and at least one
// well-formed template entry with a known strategy.
func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported schema_version %q, expected %q", m.SchemaVersion, ManifestSchemaVersion)
	}
	if m.Preset.ID == "" {
		return fmt.Errorf("preset.id is required")
	}
	if m.Preset.Name == "" {
		return fmt.Errorf("preset.name is required")
	}
	if m.Preset.Version == "" {
		return fmt.Errorf("preset.version is required")
	}
	if len(m.Provides.Templates) == 0 {
		return fmt.Errorf("provides.templates must have at least one entry")
	}
	for i, t := range m.Provides.Templates {
		if t.Type != "template" {
			return fmt.Errorf("provides.templates[%d].type must be %q, got %q", i, "template", t.Type)
		}
		if t.Name == "" {
			return fmt.Errorf("provides.templates[%d].name is required", i)
		}
		if t.File == "" {
			return fmt.Errorf("provides.templates[%d].file is required", i)
		}
		if t.Strategy != "" && !t.Strategy.Valid() {
			return fmt.Errorf("provides.templates[%d].strategy %q is not one of replace|prepend|append|wrap", i, t.Strategy)
		}
	}
	return nil
}
