package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The AGENTS.md bootstrap exists for DeepSeek Harness: the harness loads
// AGENTS.md from the project root every session, and unlike Claude Code
// it has no PreToolUse hook, so the phase whitelist binds by discipline.
// mulix merges one sentinel-wrapped section stating that discipline and
// where the skills live. The markers make the section idempotent to
// install and refreshable later without touching any content the user
// wrote around it.

const (
	agentsBootstrapBegin = "<!-- mulix:begin (do not edit between these markers) -->"
	agentsBootstrapEnd   = "<!-- mulix:end -->"
	agentsBootstrapFile  = "AGENTS.md"
)

// agentsBootstrapSection renders mulix's AGENTS.md section for a project
// whose skills are installed under skillsDir. It carries no trailing
// newline, so the merge controls the file's exact line endings.
func agentsBootstrapSection(skillsDir string) string {
	lines := []string{
		"",
		"# mulix — spec-driven workflow controls",
		"",
		"This project is managed by mulix. Its skills live in `" + skillsDir + "/`",
		"(`mulix-using` is the entry point; the embedded skills run inside the",
		"phases).",
		"",
		"1. Run `mulix state show` before any work — never infer the phase from",
		"   conversation history.",
		"2. Invoke the active phase's skill before acting in that phase.",
		"3. This host has no PreToolUse hook; follow phase discipline for all",
		"   writes. CLI transitions check recorded evidence, not authentic human",
		"   consent or proof of test execution. A failed guard is blocking.",
		"   Transition --force skips readiness only, never an illegal event;",
		"   do not use it to conceal missing consent or verification.",
		"   On Claude Code the hook covers only Write/Edit, not Bash, MCP,",
		"   NotebookEdit or other writers. Missing paths and hook errors fail",
		"   open. These are workflow controls, not a security sandbox.",
		"4. Keep every write inside the current phase's artifact paths even",
		"   though nothing technical stops you: `docs/specs/<change>/`",
		"   (specify/clarify), `docs/changes/<change>/` (tasks/verify), and",
		"   `.mulix/.runtime/<change>/` (design docs, build workspace).",
	}
	return agentsBootstrapBegin + "\n" + strings.Join(lines, "\n") + "\n" + agentsBootstrapEnd
}

// mergeAgentsBootstrap merges mulix's section into root/AGENTS.md. An
// existing section is replaced in place; without one the section is
// appended to the user's content (or becomes the whole file when the file
// doesn't exist yet). Content outside the markers is never modified. It
// reports whether the file changed.
func mergeAgentsBootstrap(root, skillsDir string) (bool, error) {
	full := filepath.Join(root, agentsBootstrapFile)
	existing, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("scaffold: reading %s: %w", full, err)
	}
	text := string(existing)
	section := agentsBootstrapSection(skillsDir)

	var out string
	begin := strings.Index(text, agentsBootstrapBegin)
	switch {
	case begin >= 0:
		relEnd := strings.Index(text[begin:], agentsBootstrapEnd)
		if relEnd < 0 {
			return false, fmt.Errorf("scaffold: %s has a mulix begin marker without an end marker; fix the file by hand", full)
		}
		end := begin + relEnd + len(agentsBootstrapEnd)
		out = text[:begin] + section + text[end:]
	case strings.TrimSpace(text) == "":
		out = section + "\n"
	default:
		out = strings.TrimRight(text, "\n") + "\n\n" + section + "\n"
	}
	if out == text {
		return false, nil
	}
	if err := writeFile(full, []byte(out)); err != nil {
		return false, fmt.Errorf("scaffold: writing %s: %w", full, err)
	}
	return true, nil
}
