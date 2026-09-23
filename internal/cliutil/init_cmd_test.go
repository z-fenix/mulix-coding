package cliutil

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setTestHome points os.UserHomeDir() at dir for the duration of the
// test, on whichever env var the current OS actually reads.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	} else {
		t.Setenv("HOME", dir)
	}
}

func TestInit_PrintsSuperpowersHintWhenNotDetected(t *testing.T) {
	dir := chdirTemp(t)
	setTestHome(t, t.TempDir())

	out, err := runPresetCLI(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init: %v (output: %s)", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("hint: the superpowers plugin wasn't detected")) {
		t.Errorf("expected a superpowers-not-detected hint, got %q", out)
	}
}

func TestInit_NoSuperpowersHintWhenDetectedViaUserScope(t *testing.T) {
	dir := chdirTemp(t)
	home := t.TempDir()
	setTestHome(t, home)

	pluginsDir := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{"version": 2, "plugins": {"superpowers@claude-plugins-official": [{"scope": "user"}]}}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, err := runPresetCLI(t, "init", "--dir", dir)
	if err != nil {
		t.Fatalf("init: %v (output: %s)", err, out)
	}
	if bytes.Contains([]byte(out), []byte("hint: the superpowers plugin wasn't detected")) {
		t.Errorf("expected no superpowers hint once it's detected, got %q", out)
	}
}
