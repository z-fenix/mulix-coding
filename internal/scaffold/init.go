package scaffold

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mulix-dev/mulix-coding/assets"
)

// InitOptions controls what mulix init writes into a target project.
type InitOptions struct {
	// Root is the target project directory (must already exist).
	Root string
	// Force overwrites files mulix owns even if they already exist and
	// differ from what mulix would write. Without Force, existing files
	// are left alone and reported as skipped.
	Force bool
}

// InitResult reports what Init actually did, so the CLI can print a
// summary instead of staying silent.
type InitResult struct {
	Written []string
	Skipped []string
}

// Init materializes .mulix/ (templates, shared constitution), .claude/skills/mulix-*,
// and the PreToolUse + SessionStart hook entries in .claude/settings.json.
// It does not create docs/changes/ or docs/specs/ — those come from
// `mulix new`, once there's an actual change to hold. It is idempotent:
// re-running without --force only fills in what's missing.
func Init(opts InitOptions) (InitResult, error) {
	var res InitResult

	if err := writeIfAbsentOrForced(&res, opts, ".mulix/memory/constitution.md", func() ([]byte, error) {
		return fs.ReadFile(assets.Templates, "templates/constitution-template.md")
	}); err != nil {
		return res, err
	}

	if err := installSkills(&res, opts); err != nil {
		return res, err
	}

	if err := installTemplates(&res, opts); err != nil {
		return res, err
	}

	if err := installClaudeHooks(&res, opts); err != nil {
		return res, err
	}

	return res, nil
}

// writeIfAbsentOrForced writes content() to root/relPath unless the file
// already exists and Force is false, in which case it's recorded as
// skipped instead of silently left alone with no report at all. Every
// file mulix writes also gets a baseline copy under .mulix/.installed/ —
// the common ancestor `mulix update` later three-way merges against.
func writeIfAbsentOrForced(res *InitResult, opts InitOptions, relPath string, content func() ([]byte, error)) error {
	full := filepath.Join(opts.Root, relPath)
	if _, err := os.Stat(full); err == nil && !opts.Force {
		res.Skipped = append(res.Skipped, relPath)
		return nil
	}
	data, err := content()
	if err != nil {
		return fmt.Errorf("scaffold: reading bundled content for %s: %w", relPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("scaffold: creating dir for %s: %w", relPath, err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return fmt.Errorf("scaffold: writing %s: %w", relPath, err)
	}
	baseFull := baselinePath(opts.Root, relPath)
	if err := os.MkdirAll(filepath.Dir(baseFull), 0o755); err != nil {
		return fmt.Errorf("scaffold: creating baseline dir for %s: %w", relPath, err)
	}
	if err := os.WriteFile(baseFull, data, 0o644); err != nil {
		return fmt.Errorf("scaffold: writing baseline for %s: %w", relPath, err)
	}
	res.Written = append(res.Written, relPath)
	return nil
}

// installSkills copies every bundled assets/skills/<name>/SKILL.md to
// .claude/skills/<name>/SKILL.md.
func installSkills(res *InitResult, opts InitOptions) error {
	entries, err := fs.ReadDir(assets.Skills, "skills")
	if err != nil {
		return fmt.Errorf("scaffold: reading bundled skills: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".claude", "skills", e.Name(), "SKILL.md"))
		srcPath := "skills/" + e.Name() + "/SKILL.md"
		err := writeIfAbsentOrForced(res, opts, rel, func() ([]byte, error) {
			return fs.ReadFile(assets.Skills, srcPath)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// installTemplates copies every bundled template into .mulix/templates/,
// so the skills' Outline steps can reference a local, project-relative
// path rather than reaching back into the mulix binary.
func installTemplates(res *InitResult, opts InitOptions) error {
	entries, err := fs.ReadDir(assets.Templates, "templates")
	if err != nil {
		return fmt.Errorf("scaffold: reading bundled templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".mulix", "templates", e.Name()))
		srcPath := "templates/" + e.Name()
		err := writeIfAbsentOrForced(res, opts, rel, func() ([]byte, error) {
			return fs.ReadFile(assets.Templates, srcPath)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// claudeSettings is the minimal shape of .claude/settings.json mulix
// reads/writes. Unknown top-level keys are preserved via Extra so init
// merges into a user's existing settings rather than clobbering them.
type claudeSettings struct {
	Hooks map[string][]hookMatcherGroup `json:"hooks,omitempty"`
	Extra map[string]json.RawMessage    `json:"-"`
}

type hookMatcherGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// installClaudeHooks merges mulix's PreToolUse hook into
// .claude/settings.json without disturbing hooks or settings the user
// already has, matching comet's settings.local.json merge behavior rather
// than overwriting the whole file.
func installClaudeHooks(res *InitResult, opts InitOptions) error {
	path := filepath.Join(opts.Root, ".claude", "settings.json")

	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("scaffold: parsing existing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("scaffold: reading %s: %w", path, err)
	}

	var hooks map[string][]hookMatcherGroup
	if existing, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(existing, &hooks); err != nil {
			return fmt.Errorf("scaffold: parsing existing hooks in %s: %w", path, err)
		}
	}
	if hooks == nil {
		hooks = map[string][]hookMatcherGroup{}
	}

	mulixHookCmd := "mulix hook"
	changed := mergeHookEntry(hooks, "PreToolUse", "Write|Edit", mulixHookCmd, opts.Force)

	if len(hooks) > 0 {
		encoded, err := json.MarshalIndent(hooks, "", "  ")
		if err != nil {
			return fmt.Errorf("scaffold: encoding hooks: %w", err)
		}
		raw["hooks"] = encoded
	}

	if !changed {
		res.Skipped = append(res.Skipped, ".claude/settings.json (hooks already present)")
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("scaffold: creating .claude dir: %w", err)
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("scaffold: encoding %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("scaffold: writing %s: %w", path, err)
	}
	res.Written = append(res.Written, ".claude/settings.json")
	return nil
}

// mergeHookEntry adds command under event/matcher if it isn't already
// present, returning whether it made a change. force also replaces a
// matcher group's hooks if only the command text differs (e.g. mulix's
// binary path changed), so re-running init after an upgrade stays in sync.
func mergeHookEntry(hooks map[string][]hookMatcherGroup, event, matcher, command string, force bool) bool {
	groups := hooks[event]
	for i, g := range groups {
		if g.Matcher != matcher {
			continue
		}
		for _, h := range g.Hooks {
			if h.Command == command {
				return false // already present, nothing to do
			}
		}
		groups[i].Hooks = append(groups[i].Hooks, hookEntry{Type: "command", Command: command})
		hooks[event] = groups
		return true
	}
	hooks[event] = append(groups, hookMatcherGroup{
		Matcher: matcher,
		Hooks:   []hookEntry{{Type: "command", Command: command}},
	})
	return true
}
