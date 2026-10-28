package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdate_ReportsMissingSessionIntegrationWithoutEditingSettings(t *testing.T) {
	root, s := sessionProject(t)
	selectForSession(t, s, "A", "Y")
	if _, err := runPresetCLI(t, "init"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".claude", "settings.json")
	old := []byte(`{"custom":"keep","hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"mulix hook"}]}]}}`)
	if err := os.WriteFile(p, old, 0644); err != nil {
		t.Fatal(err)
	}
	contextBefore, _ := os.ReadFile(filepath.Join(root, ".mulix", "context.json"))
	for _, args := range [][]string{{"update"}, {"update", "--force"}} {
		out, err := runPresetCLI(t, args...)
		if err != nil || !strings.Contains(out, "mulix init --host claude") {
			t.Fatalf("upgrade guidance: %s %v", out, err)
		}
		after, _ := os.ReadFile(p)
		if string(after) != string(old) {
			t.Fatal("update edited settings")
		}
		mode, _ := os.ReadFile(filepath.Join(root, ".mulix", "context.json"))
		if string(mode) != string(contextBefore) {
			t.Fatal("update rewrote context")
		}
	}
}
