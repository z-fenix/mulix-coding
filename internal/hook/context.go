package hook

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

// Windows trims trailing dots/spaces even for components not created yet.
// Reject these ambiguous spellings before an unrestricted build can allow them.
func ambiguousWindowsPath(p string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	for _, part := range strings.Split(strings.ReplaceAll(p, `\`, "/"), "/") {
		if part == "." || part == ".." {
			continue
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return true
		}
	}
	return false
}

// DecideContext adds control-file protection without changing phase whitelists.
func DecideContext(root string, resolved sessionctx.Resolved, req Request) Decision {
	if !gatedTools[req.ToolName] || req.ToolInput.FilePath == "" {
		return Decision{Allow: true}
	}
	if resolved.Mode == sessionctx.Session {
		if ambiguousWindowsPath(req.ToolInput.FilePath) {
			return Decision{Reason: "Windows trailing-dot or trailing-space path aliases are ambiguous.", Next: "Use canonical path components without trailing dots or spaces."}
		}
		rel := relativeSlash(root, req.ToolInput.FilePath)
		for _, p := range []string{layout.ContextFile, ".mulix/active", layout.SessionsDir, layout.LocksDir} {
			if isUnder(rel, p) {
				return Decision{Reason: fmt.Sprintf("%s is mulix control metadata", rel), Next: "Use mulix context / state select rather than editing control files."}
			}
		}
	}
	if resolved.State == nil {
		return Decision{Allow: true}
	}
	d := Decide(root, *resolved.State, req)
	if !d.Allow {
		d.Reason = fmt.Sprintf("change=%s phase=%s source=%s: %s", resolved.Change, resolved.Phase, resolved.Source, d.Reason)
	}
	return d
}
