package hook

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func TestSessionEnvLine_QuotesOpaqueIDWithoutExecuting(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	id := sessionctx.Identity{Value: "A'; touch marker; ' b", Present: true}
	line, err := SessionEnvLine(id)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cmd := exec.Command("bash", "-c", line+"printf '%s' \"$MULIX_SESSION_ID\"")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != id.Value {
		t.Fatalf("roundtrip=%q err=%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatal("session ID was executed", err)
	}
}
func TestSessionStart_RenderHasOnlyAdditionalContext(t *testing.T) {
	data, err := RenderSessionStart("use explicit identity")
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		HookSpecificOutput map[string]string `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.HookSpecificOutput["hookEventName"] != "SessionStart" || out.HookSpecificOutput["additionalContext"] != "use explicit identity" || len(out.HookSpecificOutput) != 2 {
		t.Fatalf("wrong startup protocol: %s", data)
	}
}
