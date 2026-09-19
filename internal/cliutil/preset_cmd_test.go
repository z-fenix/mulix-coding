package cliutil

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// runPresetCLI executes NewRootCmd() with args against cwd (which callers
// must chdir into a temp project root first, since preset subcommands
// resolve state.FindRoot(".") from the process's working directory, not a
// --dir flag). Every mulix subcommand prints via fmt.Print* straight to
// the process's stdout (not cmd.OutOrStdout()), matching the rest of this
// CLI's existing style, so output is captured by redirecting os.Stdout
// rather than cmd.SetOut.
func runPresetCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs(args)

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	runErr := cmd.Execute()

	w.Close()
	os.Stdout = origStdout
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("reading captured stdout: %v", readErr)
	}
	return string(out), runErr
}

// chdirTemp creates a temp dir, chdirs into it, and restores the original
// cwd on test cleanup, so parallel packages/tests aren't affected by one
// test's process-wide chdir.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
	return dir
}

func initMulixRoot(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".mulix", "state"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
}

func TestPresetList_EmptyRegistryReportsNoPresets(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	out, err := runPresetCLI(t, "preset", "list")
	if err != nil {
		t.Fatalf("preset list: %v (output: %s)", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("no presets installed")) {
		t.Errorf("expected a 'no presets installed' message, got %q", out)
	}
}

func TestPresetAddLocalDirThenListThenRemove(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `schema_version: "1.0"
preset:
  id: "cli-test-preset"
  name: "CLI Test Preset"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`
	if err := os.WriteFile(filepath.Join(src, "preset.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "templates", "spec-template.md"), []byte("cli test content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if out, err := runPresetCLI(t, "preset", "add", src); err != nil {
		t.Fatalf("preset add: %v (output: %s)", err, out)
	}

	out, err := runPresetCLI(t, "preset", "list")
	if err != nil {
		t.Fatalf("preset list: %v (output: %s)", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("cli-test-preset")) {
		t.Errorf("expected 'cli-test-preset' in list output, got %q", out)
	}

	out, err = runPresetCLI(t, "preset", "resolve", "spec-template")
	if err != nil {
		t.Fatalf("preset resolve: %v (output: %s)", err, out)
	}
	if out != "cli test content" {
		t.Errorf("expected resolve to return the preset's content, got %q", out)
	}

	if out, err := runPresetCLI(t, "preset", "remove", "cli-test-preset", "--purge"); err != nil {
		t.Fatalf("preset remove: %v (output: %s)", err, out)
	}

	out, err = runPresetCLI(t, "preset", "list")
	if err != nil {
		t.Fatalf("preset list: %v (output: %s)", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("no presets installed")) {
		t.Errorf("expected preset list to be empty after remove, got %q", out)
	}
}

func TestPresetEnableDisableSetPriority(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `schema_version: "1.0"
preset:
  id: "toggle-preset"
  name: "Toggle Preset"
  version: "1.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`
	if err := os.WriteFile(filepath.Join(src, "preset.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "templates", "spec-template.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := runPresetCLI(t, "preset", "add", src); err != nil {
		t.Fatalf("preset add: %v", err)
	}

	if _, err := runPresetCLI(t, "preset", "disable", "toggle-preset"); err != nil {
		t.Fatalf("preset disable: %v", err)
	}
	out, err := runPresetCLI(t, "preset", "list")
	if err != nil {
		t.Fatalf("preset list: %v", err)
	}
	if bytes.Contains([]byte(out), []byte("toggle-preset")) {
		t.Errorf("expected disabled preset to be hidden from default list, got %q", out)
	}

	out, err = runPresetCLI(t, "preset", "list", "--all")
	if err != nil {
		t.Fatalf("preset list --all: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("disabled")) {
		t.Errorf("expected --all to show the disabled preset, got %q", out)
	}

	if _, err := runPresetCLI(t, "preset", "set-priority", "toggle-preset", "1"); err != nil {
		t.Fatalf("preset set-priority: %v", err)
	}
	if _, err := runPresetCLI(t, "preset", "enable", "toggle-preset"); err != nil {
		t.Fatalf("preset enable: %v", err)
	}
	out, err = runPresetCLI(t, "preset", "info", "toggle-preset")
	if err != nil {
		t.Fatalf("preset info: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("priority:      1")) {
		t.Errorf("expected priority 1 in info output, got %q", out)
	}
}

func TestPresetCatalogAddListRemove(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	out, err := runPresetCLI(t, "preset", "catalog", "list")
	if err != nil {
		t.Fatalf("preset catalog list: %v (output: %s)", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("default")) {
		t.Errorf("expected the bundled 'default' catalog to always be listed, got %q", out)
	}

	if _, err := runPresetCLI(t, "preset", "catalog", "add", "community", "https://example.com/catalog.json"); err != nil {
		t.Fatalf("preset catalog add: %v", err)
	}
	out, err = runPresetCLI(t, "preset", "catalog", "list")
	if err != nil {
		t.Fatalf("preset catalog list: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("community")) {
		t.Errorf("expected 'community' catalog in list, got %q", out)
	}

	if _, err := runPresetCLI(t, "preset", "catalog", "remove", "community"); err != nil {
		t.Fatalf("preset catalog remove: %v", err)
	}
	out, err = runPresetCLI(t, "preset", "catalog", "list")
	if err != nil {
		t.Fatalf("preset catalog list: %v", err)
	}
	if bytes.Contains([]byte(out), []byte("community")) {
		t.Errorf("expected 'community' catalog to be removed, got %q", out)
	}
}
