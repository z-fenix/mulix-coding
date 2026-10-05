package scaffold

import (
	"fmt"
	"strings"
)

// Host is an agent host mulix can install project skills for. The bundled
// content itself is host-agnostic (SKILL.md directories); a host decides
// which project directory that content is copied into and which host
// specific integrations apply (the PreToolUse hook, an instruction-file
// bootstrap section).
type Host string

const (
	// HostClaude is Claude Code. Skills go to .claude/skills/, and init
	// merges the PreToolUse hook into .claude/settings.json.
	HostClaude Host = "claude"
	// HostDSH is DeepSeek Harness. Skills go to .dsh/skills/ — the
	// highest-priority root of the harness's local skill discovery — and
	// init merges a bootstrap section into the project's AGENTS.md, which
	// the harness loads every session. DSH has no PreToolUse hook, so the
	// guard checks inside `mulix state transition` are the only automated
	// enforcement there.
	HostDSH Host = "dsh"
)

// SkillsDirPlaceholder appears in mulix's own skill bodies wherever they
// state the project skills directory. Init and update replace it with the
// installing host's skills directory, so an installed skill names a path
// that actually exists in that project. The embedded superpowers skills
// are copied verbatim and never contain the placeholder.
const SkillsDirPlaceholder = "{{SKILLS_DIR}}"

// allHosts lists every host mulix knows, in a stable order.
var allHosts = []Host{HostClaude, HostDSH}

// skillsDir returns the root-relative project skills directory for h.
func (h Host) skillsDir() string {
	switch h {
	case HostDSH:
		return ".dsh/skills"
	default:
		return ".claude/skills"
	}
}

// installsHook reports whether init merges the PreToolUse hook for h.
// The hook is a Claude Code integration; no other host runs it.
func (h Host) installsHook() bool {
	return h == HostClaude
}

// writesBootstrap reports whether init merges a section into the host's
// project instruction file. Only DSH gets one: it loads AGENTS.md every
// session, and without the hook the phase whitelist binds by discipline,
// which needs the extra anchor.
func (h Host) writesBootstrap() bool {
	return h == HostDSH
}

// ParseHosts parses --host flag values: each value may name one host or a
// comma-separated list, and the flag may repeat. Matching is
// case-insensitive and whitespace-tolerant; duplicates collapse while
// keeping first-seen order.
func ParseHosts(values []string) ([]Host, error) {
	var hosts []Host
	seen := map[Host]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" {
				continue
			}
			h := Host(part)
			if !validHost(h) {
				return nil, fmt.Errorf("scaffold: unknown host %q (valid hosts: claude, dsh)", part)
			}
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	return hosts, nil
}

func validHost(h Host) bool {
	for _, known := range allHosts {
		if h == known {
			return true
		}
	}
	return false
}

// normalizeHosts validates a Hosts option and fills its zero value with
// the historical default (Claude Code alone), so Init and Update stay
// backward compatible for callers that don't set Hosts.
func normalizeHosts(hosts []Host) ([]Host, error) {
	if len(hosts) == 0 {
		return []Host{HostClaude}, nil
	}
	var out []Host
	seen := map[Host]bool{}
	for _, h := range hosts {
		if !validHost(h) {
			return nil, fmt.Errorf("scaffold: unknown host %q (valid hosts: claude, dsh)", h)
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out, nil
}
