package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "preset.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

const validManifest = `
schema_version: "1.0"
preset:
  id: "lean"
  name: "Lean Workflow"
  version: "1.0.0"
  description: "minimal"
requires:
  mulix_version: ">=0.1.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
      replaces: "spec-template"
tags:
  - "lean"
`

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Preset.ID != "lean" {
		t.Errorf("expected id 'lean', got %q", m.Preset.ID)
	}
	if len(m.Provides.Templates) != 1 || m.Provides.Templates[0].Name != "spec-template" {
		t.Errorf("expected one spec-template entry, got %+v", m.Provides.Templates)
	}
	if m.Provides.Templates[0].EffectiveStrategy() != StrategyReplace {
		t.Errorf("expected default strategy 'replace', got %q", m.Provides.Templates[0].EffectiveStrategy())
	}
}

func TestLoadManifest_UnknownFieldFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifest+"\nunknown_field: true\n")

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected LoadManifest to reject an unknown top-level field")
	}
}

func TestLoadManifest_WrongSchemaVersionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	bad := `
schema_version: "2.0"
preset:
  id: "x"
  name: "X"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`
	path := writeManifest(t, dir, bad)

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected LoadManifest to reject an unsupported schema_version")
	}
}

func TestLoadManifest_NoTemplatesFailsClosed(t *testing.T) {
	dir := t.TempDir()
	bad := `
schema_version: "1.0"
preset:
  id: "x"
  name: "X"
  version: "1.0.0"
provides:
  templates: []
`
	path := writeManifest(t, dir, bad)

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected LoadManifest to reject a manifest with no template entries")
	}
}

func TestLoadManifest_InvalidStrategyFailsClosed(t *testing.T) {
	dir := t.TempDir()
	bad := `
schema_version: "1.0"
preset:
  id: "x"
  name: "X"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
      strategy: "overwrite"
`
	path := writeManifest(t, dir, bad)

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected LoadManifest to reject an invalid strategy")
	}
}
