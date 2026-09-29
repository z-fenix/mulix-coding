// Package layout names every on-disk location mulix reads or writes,
// relative to the project root. It has no dependencies of its own, so the
// state, guard, hook, and scaffold packages can all share one definition
// without import cycles.
package layout

import "path"

// SpecsDir holds each change's requirements doc (docs/specs/<change>/
// spec.md). It's kept apart from ChangesDir because a spec stays useful
// as a reference long after the change that wrote it is archived.
const SpecsDir = "docs/specs"

// ChangesDir holds each change's spec-driven process artifacts
// (docs/changes/<change>/tasks.md, report.md).
const ChangesDir = "docs/changes"

// RuntimeDir holds everything mulix and its embedded superpowers skills
// produce while a change runs: the state file, the brainstorming design
// doc, the implementation plan, visual-companion sessions, and the
// build's execution records. One subdirectory per change, plus Shared.
const RuntimeDir = ".mulix/.runtime"

// Shared is the RuntimeDir subdirectory for output that belongs to no
// single change (the embedded skills fall back to it when no change is
// active). It is never a change id.
const Shared = "_shared"

// Subdirectories of a change's runtime directory.
const (
	// StateFile is mulix's own phase state. Never hand-edited.
	StateFile = "state.yaml"
	// DesignDir holds the design doc approved in the design phase.
	DesignDir = "specs"
	// BrainstormDir holds visual-companion sessions (design phase). They
	// carry a session key, so init git-ignores them.
	BrainstormDir = "brainstorm"
	// SddDir holds per-plan execution workspaces — ledger, task briefs,
	// task reports, test logs, review packages — written by the build
	// phase's plan executor. tasks.md is the plan, so its workspace is
	// SddDir/tasks/ (disambiguated by a plan-path marker on collision).
	SddDir = "sdd"
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
