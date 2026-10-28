package hook

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func SessionArgument(id sessionctx.Identity) (string, error) {
	if err := sessionctx.ValidateIdentity(id); err != nil {
		return "", err
	}
	if !id.Present {
		return "", fmt.Errorf("no session identity supplied")
	}
	return "'" + strings.ReplaceAll(id.Value, "'", "'\"'\"'") + "'", nil
}
func SessionEnvLine(id sessionctx.Identity) (string, error) {
	quoted, err := SessionArgument(id)
	if err != nil {
		return "", err
	}
	return "export MULIX_SESSION_ID=" + quoted + "\n", nil
}
func RenderSessionStart(additionalContext string) ([]byte, error) {
	return json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": "SessionStart", "additionalContext": additionalContext}})
}
