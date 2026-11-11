// Package hook implements Claude Code's PreToolUse phase whitelist for
// Write/Edit calls. It does not gate Bash, MCP, NotebookEdit or other tools
// that can write files. The CLI fails open on hook errors; this is workflow
// control, not an unbypassable filesystem sandbox.
package hook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
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
	SessionID     *string   `json:"session_id,omitempty"`
	HookEventName string    `json:"hook_event_name"`
	ToolName      string    `json:"tool_name"`
	ToolInput     ToolInput `json:"tool_input"`
	CWD           string    `json:"cwd"`
}

// ParseRequest decodes a PreToolUse request from r (normally stdin).
func ParseRequest(r io.Reader) (Request, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Request{}, fmt.Errorf("hook: decoding request: %w", err)
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Request{}, fmt.Errorf("hook: decoding request: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Request{}, err
	}
	if fields == nil || bytes.Equal(bytes.TrimSpace(fields["session_id"]), []byte("null")) {
		return Request{}, fmt.Errorf("hook: expected an object with an absent or string session_id, not null")
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

// gatedTools are the only tool names mulix inspects. Other tools are
// allowed unconditionally, including tools capable of filesystem writes.
var gatedTools = map[string]bool{"Write": true, "Edit": true}

// runtimeWritable contains only retained change artifacts. Transient service
// files and verification output belong in the resolved system scratch directory.
// state.yaml stays CLI-only; this is a Write/Edit policy, not a sandbox.
var runtimeWritable = map[flow.Phase][]string{
	flow.PhaseDesign: {layout.DesignDir},
	flow.PhaseBuild:  {layout.SddDir},
}

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

	if isUnder(rel, layout.RuntimeDir) {
		return decideRuntime(s, rel)
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

// decideRuntime permits only phase-owned retained artifacts. Legacy shared
// output and another change's directory are not writable destinations.
func decideRuntime(s flow.State, rel string) Decision {
	own := layout.ChangeRuntimeDir(s.Change)
	if !isUnder(rel, own) {
		return Decision{
			Allow:  false,
			Reason: fmt.Sprintf("%s belongs to another change's runtime directory; the active change is %q.", rel, s.Change),
			Next:   "Use `mulix state select <change>` if you meant to work on that change.",
		}
	}
	if samePath(rel, path.Join(own, layout.StateFile)) {
		return Decision{
			Allow:  false,
			Reason: fmt.Sprintf("%s is mulix's own state file and must not be edited directly.", rel),
			Next:   "Use `mulix state set` / `mulix state transition <event>` (which runs guards) instead of editing state files by hand.",
		}
	}
	for _, sub := range runtimeWritable[s.Phase] {
		if isUnder(rel, path.Join(own, sub)) {
			return Decision{Allow: true}
		}
	}
	var dirs []string
	for _, sub := range runtimeWritable[s.Phase] {
		dirs = append(dirs, path.Join(own, sub))
	}
	reason := fmt.Sprintf("Phase %q permits no writes under %s.", s.Phase, own)
	if len(dirs) > 0 {
		reason = fmt.Sprintf("Phase %q only permits runtime writes under %s; %s is outside that.", s.Phase, strings.Join(dirs, ", "), rel)
	}
	return Decision{
		Allow:  false,
		Reason: reason,
		Next:   "Retained runtime artifacts belong to their phase: design docs under specs/ in design, execution workspaces under sdd/ in build. Query mulix context show --field scratch-dir for temporary companion, diagnostic, verification or migration output.",
	}
}

// allowedPrefixes returns the set of slash-separated, root-relative path
// prefixes writable in s.Phase outside .mulix/.runtime/, or
// unrestricted=true when every path is writable (the build phase: that's
// where source code and tests actually get written).
//
// The spec (docs/specs/<change>/spec.md, the requirements doc) and the
// change's process artifacts (docs/changes/<change>/tasks.md, report.md)
// live in two separate directories, so which one is writable depends on
// the phase: specify and clarify edit spec.md, tasks and verify write the
// change directory. The design phase writes only under .mulix/.runtime/
// (see runtimeWritable) — its output is the design doc, not docs/.
func allowedPrefixes(s flow.State) (prefixes []string, unrestricted bool) {
	switch s.Phase {
	case flow.PhaseSpecify, flow.PhaseClarify:
		return []string{specDirFor(s)}, false
	case flow.PhaseTasks, flow.PhaseVerify:
		return []string{changeDirFor(s)}, false
	case flow.PhaseBuild:
		return nil, true
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
	return layout.SpecsDir + "/" + s.Change
}

// changeDirFor returns the change's process-artifact directory,
// docs/changes/<change>/. It's fixed by convention rather than read from
// state, so a mis-set tasks_path can't widen what the phase may write.
func changeDirFor(s flow.State) string {
	return layout.ChangesDir + "/" + s.Change
}

// relativeSlash makes p relative to root (if p is absolute) and normalizes
// to forward slashes for prefix comparisons. If p is already relative, it
// is assumed to be root-relative, matching what Claude Code's hooks
// typically pass for file_path.
func relativeSlash(root, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	if rel, err := filepath.Rel(canonicalPath(root), canonicalPath(p)); err == nil {
		p = rel
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// Write targets may not exist yet. Resolve the nearest existing ancestor so
// Windows short names and long names do not identify different policy roots.
func canonicalPath(p string) string {
	current := filepath.Clean(p)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return p
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isUnder reports whether rel is exactly prefix or nested under it, doing
// a segment-aware comparison so "specs/1-foo" does not falsely match
// prefix "specs/1".
func isUnder(rel, prefix string) bool {
	prefix = strings.TrimSuffix(filepath.ToSlash(prefix), "/")
	if runtime.GOOS == "windows" {
		rel, prefix = strings.ToLower(rel), strings.ToLower(prefix)
	}
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}
