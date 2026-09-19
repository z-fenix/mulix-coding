package preset

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPresetSource(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `schema_version: "1.0"
preset:
  id: "` + id + `"
  name: "Test Preset"
  version: "1.2.3"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`
	if err := os.WriteFile(filepath.Join(dir, "preset.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile preset.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "spec-template.md"), []byte("preset spec template"), 0o644); err != nil {
		t.Fatalf("WriteFile spec-template.md: %v", err)
	}
}

func TestInstallFromDir_CopiesAndRegisters(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	writeTestPresetSource(t, src, "myPreset")

	res, err := InstallFromDir(root, src)
	if err != nil {
		t.Fatalf("InstallFromDir: %v", err)
	}
	if res.ID != "myPreset" || res.Version != "1.2.3" {
		t.Errorf("unexpected result: %+v", res)
	}

	if _, err := os.Stat(filepath.Join(InstallDir(root, "myPreset"), "templates", "spec-template.md")); err != nil {
		t.Errorf("expected installed template file to exist: %v", err)
	}

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	entry, ok := reg.Presets["myPreset"]
	if !ok {
		t.Fatal("expected 'myPreset' to be registered")
	}
	if entry.Priority != DefaultPriority {
		t.Errorf("expected default priority %d, got %d", DefaultPriority, entry.Priority)
	}
	if !entry.Enabled {
		t.Error("expected a freshly installed preset to be enabled")
	}
}

func TestInstallFromDir_ReinstallPreservesPriorityAndEnabled(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	writeTestPresetSource(t, src, "myPreset")

	if _, err := InstallFromDir(root, src); err != nil {
		t.Fatalf("first InstallFromDir: %v", err)
	}
	if err := SetPriority(root, "myPreset", 1); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	if err := SetEnabled(root, "myPreset", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	if _, err := InstallFromDir(root, src); err != nil {
		t.Fatalf("second InstallFromDir: %v", err)
	}

	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	entry := reg.Presets["myPreset"]
	if entry.Priority != 1 {
		t.Errorf("expected re-install to preserve priority 1, got %d", entry.Priority)
	}
	if entry.Enabled {
		t.Error("expected re-install to preserve disabled state")
	}
}

func TestInstallFromDir_RejectsMissingManifest(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir() // no preset.yml

	if _, err := InstallFromDir(root, src); err == nil {
		t.Fatal("expected InstallFromDir to fail without a preset.yml")
	}
}

func buildTestZip(t *testing.T, id string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manifest := `schema_version: "1.0"
preset:
  id: "` + id + `"
  name: "Zip Preset"
  version: "2.0.0"
provides:
  templates:
    - type: "template"
      name: "spec-template"
      file: "templates/spec-template.md"
`
	// GitHub-style archives wrap everything in one top-level directory;
	// exercise that wrapper here so findManifestDir's fallback is covered.
	prefix := "repo-main/"
	writeZipFile(t, w, prefix+"preset.yml", manifest)
	writeZipFile(t, w, prefix+"templates/spec-template.md", "zip preset spec template")

	if err := w.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}
	return buf.Bytes()
}

func writeZipFile(t *testing.T, w *zip.Writer, name, content string) {
	t.Helper()
	f, err := w.Create(name)
	if err != nil {
		t.Fatalf("creating zip entry %s: %v", name, err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatalf("writing zip entry %s: %v", name, err)
	}
}

func TestInstallFromURL_DownloadsAndExtractsZip(t *testing.T) {
	root := t.TempDir()
	zipData := buildTestZip(t, "zipPreset")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	res, err := InstallFromURL(root, server.URL, "")
	if err != nil {
		t.Fatalf("InstallFromURL: %v", err)
	}
	if res.ID != "zipPreset" || res.Version != "2.0.0" {
		t.Errorf("unexpected result: %+v", res)
	}

	data, err := os.ReadFile(filepath.Join(InstallDir(root, "zipPreset"), "templates", "spec-template.md"))
	if err != nil {
		t.Fatalf("reading installed template: %v", err)
	}
	if string(data) != "zip preset spec template" {
		t.Errorf("unexpected installed content: %q", data)
	}
}

func TestInstallFromURL_RejectsHTTP(t *testing.T) {
	root := t.TempDir()
	if _, err := InstallFromURL(root, "http://example.com/preset.zip", ""); err == nil {
		t.Fatal("expected InstallFromURL to reject a non-HTTPS, non-localhost URL")
	}
}

func TestInstallFromURL_SHA256MismatchFails(t *testing.T) {
	root := t.TempDir()
	zipData := buildTestZip(t, "zipPreset")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	wrongHash := hex.EncodeToString(sha256.New().Sum(nil))
	if _, err := InstallFromURL(root, server.URL, wrongHash); err == nil {
		t.Fatal("expected InstallFromURL to reject a sha256 mismatch")
	}
}

func TestInstallFromURL_SHA256MatchSucceeds(t *testing.T) {
	root := t.TempDir()
	zipData := buildTestZip(t, "zipPreset")
	sum := sha256.Sum256(zipData)
	correctHash := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	if _, err := InstallFromURL(root, server.URL, correctHash); err != nil {
		t.Fatalf("InstallFromURL with matching sha256: %v", err)
	}
}
