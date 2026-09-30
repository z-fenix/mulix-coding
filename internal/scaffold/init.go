package scaffold

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/z-fenix/mulix-coding/assets"
	"github.com/z-fenix/mulix-coding/internal/layout"
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

// runtimeGitignore keeps the parts of .mulix/.runtime/ that must never be
// committed out of git: visual-companion sessions (they hold a session
// key) and the shared scratch directory. State, design docs, and
// execution workspaces stay trackable — they're the change's record.
const runtimeGitignore = "# Written by mulix init.\n*/" + layout.BrainstormDir + "/\n/" + layout.Shared + "/\n"

// Init materializes .mulix/ (templates, shared constitution, the runtime
// directory), .claude/skills/ (mulix's phase skills plus every embedded
// superpowers skill), and the PreToolUse hook entry in
// .claude/settings.json. It does not create docs/changes/ or docs/specs/
// — those come from `mulix new`, once there's an actual change to hold.
// It is idempotent: re-running without --force only fills in what's
// missing.
func Init(opts InitOptions) (InitResult, error) {
	var res InitResult

	if err := writeIfAbsentOrForced(&res, opts, ".mulix/memory/constitution.md", 0o644, func() ([]byte, error) {
		return fs.ReadFile(assets.Templates, "templates/constitution-template.md")
	}); err != nil {
		return res, err
	}

	files, err := managedFiles()
	if err != nil {
		return res, fmt.Errorf("scaffold: reading bundled content: %w", err)
	}
	for _, f := range files {
		if err := writeIfAbsentOrForced(&res, opts, f.Rel, f.Mode(), func() ([]byte, error) { return f.Data, nil }); err != nil {
			return res, err
		}
	}

	if err := writeIfAbsentOrForced(&res, opts, path.Join(layout.RuntimeDir, ".gitignore"), 0o644, func() ([]byte, error) {
		return []byte(runtimeGitignore), nil
	}); err != nil {
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
func writeIfAbsentOrForced(res *InitResult, opts InitOptions, relPath string, mode fs.FileMode, content func() ([]byte, error)) error {
	full := filepath.Join(opts.Root, filepath.FromSlash(relPath))
	if _, err := os.Stat(full); err == nil && !opts.Force {
		res.Skipped = append(res.Skipped, relPath)
		return nil
	}
	data, err := content()
	if err != nil {
		return fmt.Errorf("scaffold: reading bundled content for %s: %w", relPath, err)
	}
	if err := writeFileMode(full, data, mode); err != nil {
		return fmt.Errorf("scaffold: writing %s: %w", relPath, err)
	}
	if err := writeFile(baselinePath(opts.Root, relPath), data); err != nil {
		return fmt.Errorf("scaffold: writing baseline for %s: %w", relPath, err)
	}
	res.Written = append(res.Written, relPath)
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
