// Package hook implements the Claude Code PreToolUse hook that enforces
// mulix's phase whitelist on every Write/Edit call. The block is enforced
// by the host tool's exit code, not by the model choosing to comply, so an
// agent cannot skip a phase's gate simply by not calling `mulix guard`.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/scaffold"
)

// ToolInput is the subset of Claude Code's tool_input payload mulix reads.
// Write and Edit both carry file_path; other fields are ignored.
type ToolInput struct {
	FilePath string `json:"file_path"`
}

// Request is the subset of Claude Code's PreToolUse hook stdin payload
// mulix needs. See https://docs.claude.com/en/docs/claude-code/hooks for
// the full schema; unknown fields are ignored (this is untrusted input
// from the host, not mulix's own state).
type Request struct {
	HookEventName string    `json:"hook_event_name"`
	ToolName      string    `json:"tool_name"`
	ToolInput     ToolInput `json:"tool_input"`
	CWD           string    `json:"cwd"`
}

// ParseRequest decodes a PreToolUse request from r (normally stdin).
func ParseRequest(r io.Reader) (Request, error) {
	var req Request
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return Request{}, fmt.Errorf("hook: decoding request: %w", err)
	}
	return req, nil
}

// Decision is the allow/block outcome for one tool call.
type Decision struct {
	Allow  bool
	Reason string
	// Next is a short remediation hint, shown only when Allow is false.
	Next string
}

// gatedTools are the only tool names mulix inspects. Anything else (Bash,
// Read, Grep, ...) is allowed unconditionally: mulix gates writes, not
// exploration or verification commands.
var gatedTools = map[string]bool{"Write": true, "Edit": true}

// runtimeDirName is the subdirectory of a change directory holding
// mulix's own state for that change (docs/changes/<change>/.runtime/). It is
// always off-limits to direct Write/Edit, in every phase: state must move
// only through guarded transitions, never a raw file edit, or the whole
// mechanism can be bypassed by editing the scoreboard instead of playing
// the game.
//
// The one carve-out is runtimeSddDirName below: during the build phase
// only, that one subdirectory is writable, since it's not state at all.
const runtimeDirName = scaffold.RuntimeDirName

// runtimeSddDirName is the subdirectory of runtimeDirName holding
// process artifacts from a delegated (subagent-driven) build execution:
// dispatch plans, task breakdowns, and review records. It is writable
// during the build phase even though the rest of .runtime/ is not,
// because a delegated build chain needs somewhere to put those artifacts
// that isn't the change's public-facing docs/changes/<change>/ files.
// state.yaml itself, and everything else directly under .runtime/, stays
// blocked in every phase including build.
const runtimeSddDirName = scaffold.RuntimeSddDirName

// Decide applies the phase whitelist to req against root/s. root is the
// project root (as found by state.FindRoot); s is the currently active
// change's state.
func Decide(root string, s flow.State, req Request) Decision {
	if !gatedTools[req.ToolName] {
		return Decision{Allow: true}
	}
	if req.ToolInput.FilePath == "" {
		return Decision{Allow: true}
	}

	rel := relativeSlash(root, req.ToolInput.FilePath)

	runtimeDir := "docs/changes/" + s.Change + "/" + runtimeDirName
	sddDir := runtimeDir + "/" + runtimeSddDirName
	if isUnder(rel, runtimeDir) && !(s.Phase == flow.PhaseBuild && isUnder(rel, sddDir)) {
		return Decision{
			Allow:  false,
			Reason: fmt.Sprintf("%s is mulix's own state file and must not be edited directly.", rel),
			Next:   "Use `mulix state transition <change> <event>` (which runs guards) instead of editing state files by hand.",
		}
	}

	allowed, unrestricted := allowedPrefixes(s)
	if unrestricted {
		return Decision{Allow: true}
	}
	if len(allowed) == 0 {
		return Decision{
			Allow:  false,
			Reason: fmt.Sprintf("Phase %q permits no file writes.", s.Phase),
			Next:   "Finish this phase's CLI step (e.g. `mulix state transition`) before writing files.",
		}
	}
	for _, prefix := range allowed {
		if prefix == "docs" && (isUnder(rel, "docs/specs") || isUnder(rel, "docs/changes")) {
			// docs/specs/<other-change>/ and docs/changes/<other-change>/
			// are both reachable through this prefix too, which would
			// defeat the point of checking specDirFor separately above
			// and would let the specify phase write into other changes'
			// process artifacts. Design docs belong directly under
			// docs/, not nested in docs/specs/ or docs/changes/.
			continue
		}
		if isUnder(rel, prefix) {
			return Decision{Allow: true}
		}
	}
	return Decision{
		Allow:  false,
		Reason: fmt.Sprintf("Phase %q only permits writes under %s; %s is outside that.", s.Phase, strings.Join(allowed, ", "), rel),
		Next:   fmt.Sprintf("If this write is legitimate, advance out of %q via `mulix state transition` first (guards enforce readiness).", s.Phase),
	}
}

// allowedPrefixes returns the set of slash-separated, root-relative path
// prefixes writable in s.Phase, or unrestricted=true when every path is
// writable (the build phase: that's where source code and tests actually
// get written).
//
// The spec (docs/specs/<change>/spec.md, the requirements doc) and the
// rest of a change's artifacts (docs/changes/<change>/plan.md, tasks.md,
// ..., the process artifacts produced while executing that spec) live in
// two separate directories, so which one is writable depends on the
// phase: specify and clarify both still edit spec.md directly, everything
// from plan onward writes into the change directory instead.
func allowedPrefixes(s flow.State) (prefixes []string, unrestricted bool) {
	switch s.Phase {
	case flow.PhaseSpecify:
		// Architectural-track design docs may also land under docs/,
		// before spec.md exists. (docs/specs and docs/changes are
		// excluded from that blanket allowance below, in the
		// prefix-matching loop.)
		return []string{specDirFor(s), "docs"}, false
	case flow.PhaseClarify:
		return []string{specDirFor(s)}, false
	case flow.PhasePlan, flow.PhaseTasks, flow.PhaseAnalyze, flow.PhaseVerify:
		return []string{changeDirFor(s)}, false
	case flow.PhaseBuild:
		return nil, true
	case flow.PhaseArchive:
		return []string{}, false
	default:
		return []string{}, false
	}
}

// specDirFor returns the directory holding the change's spec.md,
// preferring the state-recorded spec path (once it exists) and falling
// back to the docs/specs/<change>/ convention before spec.md has been
// written (relevant early in the specify phase, before spec_path is
// recorded).
func specDirFor(s flow.State) string {
	if s.SpecPath != "" {
		return path.Dir(filepath.ToSlash(s.SpecPath))
	}
	return "docs/specs/" + s.Change
}

// changeDirFor returns the change's process-artifact directory,
// preferring the state-recorded plan path (once it exists, from the plan
// phase onward) and falling back to the docs/changes/<change>/
// convention before plan.md has been written.
func changeDirFor(s flow.State) string {
	if s.PlanPath != "" {
		return path.Dir(filepath.ToSlash(s.PlanPath))
	}
	return "docs/changes/" + s.Change
}

// relativeSlash makes p relative to root (if p is absolute) and normalizes
// to forward slashes for prefix comparisons. If p is already relative, it
// is assumed to be root-relative, matching what Claude Code's hooks
// typically pass for file_path.
func relativeSlash(root, p string) string {
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(root, p); err == nil {
			p = rel
		}
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// isUnder reports whether rel is exactly prefix or nested under it, doing
// a segment-aware comparison so "specs/1-foo" does not falsely match
// prefix "specs/1".
func isUnder(rel, prefix string) bool {
	prefix = strings.TrimSuffix(filepath.ToSlash(prefix), "/")
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}
