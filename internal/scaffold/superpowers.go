package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// superpowersPluginPrefix is the key prefix Claude Code's plugin installer
// uses for this plugin's entries in installed_plugins.json, e.g.
// "<prefix>@claude-plugins-official". Matching on the prefix rather than a
// full key keeps detection working across marketplaces.
const superpowersPluginPrefix = "superpowers@"

// installedPluginsFile is the shape of Claude Code's user-scope
// ~/.claude/plugins/installed_plugins.json that DetectSuperpowers reads.
// Only the key set is needed, so the per-entry install records are left
// as untyped JSON.
type installedPluginsFile struct {
	Plugins map[string]json.RawMessage `json:"plugins"`
}

// DetectSuperpowers reports whether a plugin appears to be installed for
// root, checking project scope first, then user scope. It is best-effort:
// any error reading or parsing either location is treated as "not found"
// rather than surfaced, since this check must never block `mulix init` —
// it only decides whether to print a hint.
func DetectSuperpowers(root string) (found bool, detail string) {
	if found, detail := detectSuperpowersProjectScope(root); found {
		return found, detail
	}
	if found, detail := detectSuperpowersUserScope(); found {
		return found, detail
	}
	return false, ""
}

// detectSuperpowersProjectScope looks for a project-scope plugin install
// under root/.claude/plugins/. Claude Code lays project-scope plugins out
// as .claude/plugins/<marketplace>/<plugin-name>/..., so it's enough to
// check for any directory named "superpowers" under .claude/plugins/.
func detectSuperpowersProjectScope(root string) (bool, string) {
	base := filepath.Join(root, ".claude", "plugins")
	entries, err := os.ReadDir(base)
	if err != nil {
		return false, ""
	}
	for _, e := range entries {
		if found, detail := findPluginDirIn(filepath.Join(base, e.Name())); found {
			return true, detail
		}
	}
	return false, ""
}

// findPluginDirIn checks dir (expected to be a marketplace directory)
// for a "superpowers" subdirectory.
func findPluginDirIn(dir string) (bool, string) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false, ""
	}
	candidate := filepath.Join(dir, "superpowers")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return true, candidate
	}
	return false, ""
}

// detectSuperpowersUserScope looks for a superpowers@<marketplace> entry
// in the current user's ~/.claude/plugins/installed_plugins.json.
func detectSuperpowersUserScope() (bool, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, ""
	}
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	var parsed installedPluginsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, ""
	}
	for key := range parsed.Plugins {
		if strings.HasPrefix(key, superpowersPluginPrefix) {
			return true, key
		}
	}
	return false, ""
}
