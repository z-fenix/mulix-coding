package hook

import "encoding/json"

// hookSpecificOutput mirrors Claude Code's PreToolUse response shape:
// https://docs.claude.com/en/docs/claude-code/hooks-guide
type hookSpecificOutput struct {
	HookEventName      string `json:"hookEventName"`
	PermissionDecision string `json:"permissionDecision,omitempty"`
	PermissionReason   string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext  string `json:"additionalContext,omitempty"`
}

type response struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

// Render encodes d as the JSON Claude Code expects on stdout. Callers are
// responsible for the process exit code: allow -> 0, block -> 2 (per
// Claude Code's PreToolUse hook contract, where a non-zero exit plus this
// JSON is what actually stops the tool call, not the JSON alone).
func Render(d Decision) ([]byte, error) {
	out := response{HookSpecificOutput: hookSpecificOutput{HookEventName: "PreToolUse"}}
	if d.Allow {
		out.HookSpecificOutput.PermissionDecision = "allow"
	} else {
		out.HookSpecificOutput.PermissionDecision = "deny"
		reason := d.Reason
		if d.Next != "" {
			reason += " Next: " + d.Next
		}
		out.HookSpecificOutput.PermissionReason = reason
	}
	return json.Marshal(out)
}

// ExitCode returns the process exit code matching d, per Claude Code's
// PreToolUse contract.
func ExitCode(d Decision) int {
	if d.Allow {
		return 0
	}
	return 2
}
