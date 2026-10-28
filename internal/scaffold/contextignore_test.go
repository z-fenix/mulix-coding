package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_SessionStartMergeIsIdempotentAndPreservesUserHooks(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	original := `{"other":"keep","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-command","timeout":0.25,"shell":"powershell","statusMessage":"mine"},{"type":"prompt","prompt":"keep my prompt","model":"user-model"}]}]}}`
	if err := os.WriteFile(p, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := Init(InitOptions{Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if string(out["other"]) != `"keep"` {
		t.Fatal("top setting lost")
	}
	var hooks map[string][]struct {
		Hooks []map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(out["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	count := 0
	userFound, promptFound := false, false
	for _, g := range hooks["SessionStart"] {
		for _, h := range g.Hooks {
			if h["command"] == "mulix hook session-start" {
				count++
				if h["timeout"] != float64(15) {
					t.Fatalf("startup timeout: %v", h)
				}
			}
			if h["command"] == "user-command" {
				userFound = h["timeout"] == 0.25 && h["shell"] == "powershell" && h["statusMessage"] == "mine"
			}
			if h["type"] == "prompt" {
				promptFound = h["prompt"] == "keep my prompt" && h["model"] == "user-model"
			}
		}
	}
	if count != 1 || !userFound || !promptFound {
		t.Fatalf("hook merge lost data: %s", data)
	}
}
func TestContextIgnore_PreservesCustomRulesAndLocalData(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".mulix", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# custom\r\n/local-user"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureContextIgnore(root)
	if err != nil || !changed {
		t.Fatalf("first: %v %v", changed, err)
	}
	before, _ := os.ReadFile(p)
	changed, err = ensureContextIgnore(root)
	if err != nil || changed {
		t.Fatalf("second: %v %v", changed, err)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) || !strings.HasPrefix(string(after), "# custom\r\n/local-user") {
		t.Fatalf("custom ignore lost: %q", after)
	}
	for _, rule := range []string{"/context.json", "/sessions/", "/.locks/"} {
		if !strings.Contains(string(after), rule) {
			t.Fatal("missing rule", rule)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mulix", "context.json")); !os.IsNotExist(err) {
		t.Fatal("ignore enabled session mode")
	}
}
