package scaffold

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setTestHome points os.UserHomeDir() at dir for the duration of the test,
// on whichever env var the current OS actually reads (HOME everywhere but
// Windows, USERPROFILE there).
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	} else {
		t.Setenv("HOME", dir)
	}
}

func TestDetectSuperpowers_NotFoundWhenNeitherScopeHasIt(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, t.TempDir())

	found, detail := DetectSuperpowers(root)
	if found {
		t.Fatalf("expected not found, got found with detail %q", detail)
	}
}

func TestDetectSuperpowers_FoundViaUserScopeInstalledPluginsJSON(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	setTestHome(t, home)

	pluginsDir := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{
  "version": 2,
  "plugins": {
    "superpowers@claude-plugins-official": [
      {"scope": "user", "version": "6.4.1"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	found, detail := DetectSuperpowers(root)
	if !found {
		t.Fatal("expected found via user-scope installed_plugins.json")
	}
	if detail != "superpowers@claude-plugins-official" {
		t.Fatalf("expected detail to name the matched plugin key, got %q", detail)
	}
}

func TestDetectSuperpowers_NotFoundWhenInstalledPluginsJSONHasOtherPlugins(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	setTestHome(t, home)

	pluginsDir := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := `{
  "version": 2,
  "plugins": {
    "frontend-design@claude-plugins-official": [
      {"scope": "user", "version": "1.0.0"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	found, _ := DetectSuperpowers(root)
	if found {
		t.Fatal("expected not found when installed_plugins.json has only unrelated plugins")
	}
}

func TestDetectSuperpowers_NotFoundWhenInstalledPluginsJSONIsMalformed(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	setTestHome(t, home)

	pluginsDir := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	found, _ := DetectSuperpowers(root)
	if found {
		t.Fatal("expected malformed installed_plugins.json to be treated as not found, not an error")
	}
}

func TestDetectSuperpowers_FoundViaProjectScopePluginDir(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, t.TempDir())

	pluginDir := filepath.Join(root, ".claude", "plugins", "claude-plugins-official", "superpowers")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	found, detail := DetectSuperpowers(root)
	if !found {
		t.Fatal("expected found via project-scope .claude/plugins/ directory")
	}
	if detail == "" {
		t.Fatal("expected a non-empty detail naming the matched path")
	}
}

func TestDetectSuperpowers_ProjectScopeTakesPrecedenceOverMissingUserScope(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, filepath.Join(t.TempDir(), "does-not-exist"))

	pluginDir := filepath.Join(root, ".claude", "plugins", "claude-plugins-official", "superpowers")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	found, _ := DetectSuperpowers(root)
	if !found {
		t.Fatal("expected project scope to be found even when the user-scope home dir doesn't exist")
	}
}
