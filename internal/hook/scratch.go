package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

// decideScratch handles the reserved temp namespace before build's ordinary
// unrestricted allowance, so another context's scratch is never ordinary output.
func decideScratch(root string, resolved sessionctx.Resolved, filePath string) (Decision, bool) {
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(root, filePath)
	}
	// Keep both spellings: resolving a symlink may erase the reserved name,
	// while an alias outside the namespace may resolve into a foreign scratch.
	temp := os.TempDir()
	canonicalTemp, target := canonicalPath(temp), canonicalPath(filePath)
	if !scratchNamespace(temp, filePath) && !scratchNamespace(canonicalTemp, filePath) && !scratchNamespace(canonicalTemp, target) {
		return Decision{}, false
	}
	deny := Decision{
		Reason: "Scratch output must belong to the current project, change and session, and match its phase.",
		Next:   "Query mulix context show --field scratch-dir; use brainstorm/ in design, verification/ or migrations/ in build/verify, or diagnosing-superpowers/ for diagnostics.",
	}
	if resolved.ScratchDir == "" || !isUnder(filepath.ToSlash(target), filepath.ToSlash(canonicalPath(resolved.ScratchDir))) {
		return deny, true
	}
	// The context root itself is an identity boundary, not an alias to follow.
	// A missing root is fine: Write may be creating the first scratch artifact.
	if info, err := os.Lstat(resolved.ScratchDir); err != nil {
		if !os.IsNotExist(err) {
			return deny, true
		}
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return deny, true
	}
	if resolved.State == nil {
		return Decision{Allow: true}, true
	}
	rel := relativeSlash(resolved.ScratchDir, filePath)
	allowed := []string{"diagnosing-superpowers"}
	switch resolved.State.Phase {
	case flow.PhaseDesign:
		allowed = append(allowed, layout.BrainstormDir)
	case flow.PhaseBuild, flow.PhaseVerify:
		allowed = append(allowed, layout.VerificationDir, layout.MigrationsDir)
	}
	for _, dir := range allowed {
		if isUnder(rel, dir) {
			return Decision{Allow: true}, true
		}
	}
	deny.Reason = fmt.Sprintf("Phase %q only permits scratch writes under %s.", resolved.State.Phase, strings.Join(allowed, ", "))
	return deny, true
}

func scratchNamespace(temp, target string) bool {
	rel, err := filepath.Rel(temp, target)
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	if runtime.GOOS == "windows" {
		first = strings.ToLower(first)
	}
	return strings.HasPrefix(first, layout.ScratchPrefix)
}
