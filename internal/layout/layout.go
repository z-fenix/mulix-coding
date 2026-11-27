// Package layout names every on-disk location mulix reads or writes,
// relative to the project root. It has no dependencies of its own, so the
// state, guard, hook, and scaffold packages can all share one definition
// without import cycles.
package layout

import (
	"path"
	"strconv"
	"strings"
)

// SpecsDir holds each change's requirements doc (docs/specs/<change>/
// spec.md). It's kept apart from ChangesDir because a spec stays useful
// as a reference long after the change that wrote it is archived.
const SpecsDir = "docs/specs"

// ChangesDir holds each change's spec-driven process artifacts
// (docs/changes/<change>/tasks.md, report.md).
const ChangesDir = "docs/changes"

// RuntimeDir holds each change's state, approved design documents, and
// retained execution records. Transient material belongs in system scratch.
const RuntimeDir = ".mulix/.runtime"

// Shared is a reserved legacy runtime name, retained for discovery and ignore
// compatibility. New output must not use it.
const Shared = "_shared"

// ScratchPrefix reserves context-owned directories under the system temp root.
const ScratchPrefix = "mulix-scratch-"

// Context metadata is project-local, separate from change-owned runtime output.
const (
	ContextFile = ".mulix/context.json"
	SessionsDir = ".mulix/sessions"
	LocksDir    = ".mulix/.locks"
)

// Subdirectories of a change's runtime directory.
const (
	// StateFile is mulix's own phase state. Never hand-edited.
	StateFile = "state.yaml"
	// DesignDir holds the design doc approved in the design phase.
	DesignDir = "specs"
	// TestsDir holds authored Karate cases and executable helpers, never output.
	TestsDir = "tests"
	// BrainstormDir holds transient visual-companion sessions in scratch.
	// The name also remains in legacy runtime ignore rules for token safety.
	BrainstormDir = "brainstorm"
	// SddDir holds per-plan execution workspaces — ledger, task briefs,
	// task reports, test logs, review packages — written by the build
	// phase's plan executor. tasks.md is the plan, so its workspace is
	// SddDir/tasks/ (disambiguated by a plan-path marker on collision).
	SddDir = "sdd"
	// VerificationDir holds transient tooling and per-run evidence in scratch.
	VerificationDir = "verification"
	// MigrationsDir holds transient relocation records in scratch.
	MigrationsDir = "migrations"
)

// ChangeRuntimeDir returns the slash-separated, root-relative runtime
// directory for change.
func ChangeRuntimeDir(change string) string {
	return path.Join(RuntimeDir, change)
}

// ChangeRuntimePath joins elems onto change's runtime directory.
func ChangeRuntimePath(change string, elems ...string) string {
	return path.Join(append([]string{ChangeRuntimeDir(change)}, elems...)...)
}

// RuntimeTestGroup classifies an authored file relative to a change's tests
// directory. It returns helpers, final, or a canonical task-N, or empty for
// unsupported files. Path validation is portable even on a Unix host.
func RuntimeTestGroup(rel string) string {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\:\x00") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return ""
		}
	}
	ext := path.Ext(rel)
	if len(parts) >= 2 && parts[0] == "helpers" && (ext == ".feature" || ext == ".js") {
		return "helpers"
	}
	if len(parts) < 3 || parts[0] != "features" || ext != ".feature" {
		return ""
	}
	if parts[1] == "final" {
		return "final"
	}
	if number, ok := strings.CutPrefix(parts[1], "task-"); ok {
		if n, err := strconv.Atoi(number); err == nil && n > 0 && strconv.Itoa(n) == number {
			return parts[1]
		}
	}
	return ""
}
