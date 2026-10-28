package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Unknown nested hook fields must survive installing a new event. In
// particular prompt/http hooks are not command hooks with an empty command.
func decodeHookExtra(data []byte, into any, keys ...string) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(data, into); err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for _, key := range keys {
		delete(raw, key)
	}
	return raw, nil
}
func encodeHookExtra(value any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, exists := raw[key]; !exists {
			raw[key] = value
		}
	}
	return json.Marshal(raw)
}
func (h *hookEntry) UnmarshalJSON(data []byte) error {
	type plain hookEntry
	var value plain
	extra, err := decodeHookExtra(data, &value, "type", "command", "timeout")
	if err != nil {
		return err
	}
	*h = hookEntry(value)
	h.Extra = extra
	return nil
}
func (h hookEntry) MarshalJSON() ([]byte, error) {
	type plain hookEntry
	return encodeHookExtra(plain(h), h.Extra)
}
func (h *hookMatcherGroup) UnmarshalJSON(data []byte) error {
	type plain hookMatcherGroup
	var value plain
	extra, err := decodeHookExtra(data, &value, "matcher", "hooks")
	if err != nil {
		return err
	}
	*h = hookMatcherGroup(value)
	h.Extra = extra
	return nil
}
func (h hookMatcherGroup) MarshalJSON() ([]byte, error) {
	type plain hookMatcherGroup
	return encodeHookExtra(plain(h), h.Extra)
}

func ensureMulixHookTimeout(hooks map[string][]hookMatcherGroup, event, command string) bool {
	changed := false
	for i := range hooks[event] {
		for j := range hooks[event][i].Hooks {
			h := &hooks[event][i].Hooks[j]
			if h.Type == "command" && h.Command == command && h.Timeout < 15 {
				h.Timeout = 15
				changed = true
			}
		}
	}
	return changed
}

// NeedsSessionHook only inspects an existing Claude installation; it never
// edits settings and does not mistake a DSH-only install for Claude.
func NeedsSessionHook(root string) (bool, error) {
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", entrySkillName, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var raw struct {
		Hooks map[string][]hookMatcherGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, err
	}
	for _, g := range raw.Hooks["SessionStart"] {
		if g.Matcher != "" {
			continue
		}
		for _, h := range g.Hooks {
			if h.Type == "command" && h.Command == "mulix hook session-start" {
				return false, nil
			}
		}
	}
	return true, nil
}
