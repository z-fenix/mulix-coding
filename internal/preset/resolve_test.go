package preset

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// fakeCoreFS builds an in-memory fs.FS rooted the way assets.Templates is:
// "templates/<name>.md" holding content.
func fakeCoreFS(content map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, c := range content {
		m["templates/"+name+".md"] = &fstest.MapFile{Data: []byte(c)}
	}
	return m
}

// installTestPreset writes a minimal preset with one template entry into
// .mulix/presets/<id>/ under root and registers it, so ResolveTemplateContent
// can find it via the registry the same way a real `preset add` would have
// left things.
func installTestPreset(t *testing.T, root, id string, priority int, entryYAML, fileRelPath, fileContent string) {
	t.Helper()
	dir := InstallDir(root, id)
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(fileRelPath)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preset.yml"), []byte(entryYAML), 0o644); err != nil {
		t.Fatalf("WriteFile preset.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileRelPath), []byte(fileContent), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", fileRelPath, err)
	}
	if err := Add(root, id, Entry{Version: "1.0.0", Enabled: true, Priority: priority, InstalledAt: time.Now().UTC()}); err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestResolveTemplateContent_NoOverridesFallsBackToCore(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	if got != "core content" {
		t.Errorf("expected core content, got %q", got)
	}
}

func TestResolveTemplateContent_ProjectOverrideWinsOverEverything(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	overrideDir := filepath.Join(root, ".mulix", "templates", "overrides")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overrideDir, "spec-template.md"), []byte("project override"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	installTestPreset(t, root, "lean", 1, `schema_version: "1.0"
preset:
  id: "lean"
  name: "Lean"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`, "templates/spec-template.md", "preset content")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	if got != "project override" {
		t.Errorf("expected project override to win, got %q", got)
	}
}

func TestResolveTemplateContent_PresetReplaceShadowsCore(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "lean", 1, `schema_version: "1.0"
preset:
  id: "lean"
  name: "Lean"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
      strategy: "replace"
`, "templates/spec-template.md", "preset replace content")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	if got != "preset replace content" {
		t.Errorf("expected preset content to shadow core, got %q", got)
	}
}

func TestResolveTemplateContent_HigherPriorityReplaceShadowsLowerPreset(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "low-priority", 10, `schema_version: "1.0"
preset:
  id: "low-priority"
  name: "Low"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`, "templates/spec-template.md", "low priority content")

	installTestPreset(t, root, "high-priority", 1, `schema_version: "1.0"
preset:
  id: "high-priority"
  name: "High"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`, "templates/spec-template.md", "high priority content")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	if got != "high priority content" {
		t.Errorf("expected higher-priority (lower number) preset to win, got %q", got)
	}
}

func TestResolveTemplateContent_AppendStrategy(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "addon", 1, `schema_version: "1.0"
preset:
  id: "addon"
  name: "Addon"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/addendum.md"
      strategy: "append"
`, "templates/addendum.md", "extra section")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	want := "core content\n\nextra section"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolveTemplateContent_PrependStrategy(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "addon", 1, `schema_version: "1.0"
preset:
  id: "addon"
  name: "Addon"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/preamble.md"
      strategy: "prepend"
`, "templates/preamble.md", "preamble section")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	want := "preamble section\n\ncore content"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolveTemplateContent_WrapStrategy(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "wrapper", 1, `schema_version: "1.0"
preset:
  id: "wrapper"
  name: "Wrapper"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/wrapper.md"
      strategy: "wrap"
`, "templates/wrapper.md", "BEFORE\n{CORE_TEMPLATE}\nAFTER")

	got, err := ResolveTemplateContent(root, "spec-template", core)
	if err != nil {
		t.Fatalf("ResolveTemplateContent: %v", err)
	}
	want := "BEFORE\ncore content\nAFTER"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolveTemplateContent_WrapWithoutPlaceholderFails(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"spec-template": "core content"})

	installTestPreset(t, root, "wrapper", 1, `schema_version: "1.0"
preset:
  id: "wrapper"
  name: "Wrapper"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/wrapper.md"
      strategy: "wrap"
`, "templates/wrapper.md", "no placeholder here")

	if _, err := ResolveTemplateContent(root, "spec-template", core); err == nil {
		t.Fatal("expected an error when a wrap layer is missing {CORE_TEMPLATE}")
	}
}

func TestResolveTemplateContent_NotFoundAnywhereFails(t *testing.T) {
	root := t.TempDir()
	core := fakeCoreFS(map[string]string{"other-template": "x"})

	if _, err := ResolveTemplateContent(root, "spec-template", core); err == nil {
		t.Fatal("expected an error when the template exists in no layer")
	}
}
