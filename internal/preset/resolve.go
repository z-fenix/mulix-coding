package preset

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// layer is one candidate source of content for a template name, ordered
// highest-priority first by the caller.
type layer struct {
	content  string
	strategy Strategy
}

// ResolveTemplateContent composes the content for a template name across
// mulix's three-tier override stack, the Go port of spec-kit's
// resolve_template_content: project overrides (always "replace") ->
// installed presets in priority order -> core bundled (coreFS, always
// "replace"). mulix has no extensions layer, so this stack is one tier
// shorter than spec-kit's four.
//
// name is the template's bare name (e.g. "spec-template"), without
// directory or ".md" suffix. coreFS is rooted such that the core template
// lives at "templates/<name>.md" (matching assets.Templates' layout).
func ResolveTemplateContent(root, name string, coreFS fs.FS) (string, error) {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("preset: invalid template name %q", name)
	}

	// Priority 1: project overrides, always "replace".
	overridePath := filepath.Join(root, ".mulix", "templates", "overrides", name+".md")
	if data, err := os.ReadFile(overridePath); err == nil {
		return string(data), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("preset: reading override %s: %w", overridePath, err)
	}

	var layers []layer

	// Priority 2: installed presets, highest priority (lowest number) first.
	reg, err := LoadRegistry(root)
	if err != nil {
		return "", err
	}
	for _, p := range ListByPriority(reg, false) {
		entry, strategy, ok, err := findPresetTemplate(root, p.ID, name)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		layers = append(layers, layer{content: entry, strategy: strategy})
		if strategy == StrategyReplace {
			break // a replace layer fully shadows everything below it
		}
	}

	// Priority 3 (mulix's equivalent of spec-kit's priority 4, since there
	// is no extensions tier): core bundled template, always "replace".
	if len(layers) == 0 || layers[len(layers)-1].strategy != StrategyReplace {
		data, err := fs.ReadFile(coreFS, "templates/"+name+".md")
		if err != nil {
			if len(layers) == 0 {
				return "", fmt.Errorf("preset: template %q not found in any layer", name)
			}
			// Composing layers (prepend/append/wrap) with no replace base
			// to land on: this mirrors common.sh's "no replace base" error.
			return "", fmt.Errorf("preset: template %q has composing layers but no replace base", name)
		}
		layers = append(layers, layer{content: string(data), strategy: StrategyReplace})
	}

	return composeLayers(layers)
}

// findPresetTemplate looks up name in preset id's manifest under root,
// returning its content, declared strategy, and whether it was found at
// all. A preset that does not declare name for this template is not an
// error; it simply contributes nothing at this layer.
func findPresetTemplate(root, id, name string) (content string, strategy Strategy, ok bool, err error) {
	dir := InstallDir(root, id)
	manifestPath := filepath.Join(dir, "preset.yml")
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return "", "", false, fmt.Errorf("preset: loading manifest for %q: %w", id, err)
	}
	for _, t := range m.Provides.Templates {
		if t.Name != name {
			continue
		}
		filePath := filepath.Join(dir, filepath.FromSlash(t.File))
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", "", false, fmt.Errorf("preset: reading %s for preset %q template %q: %w", t.File, id, name, err)
		}
		return string(data), t.EffectiveStrategy(), true, nil
	}
	return "", "", false, nil
}

// composeLayers folds layers (ordered highest-priority first) down to a
// single string. The nearest "replace" layer (scanning from the top) is
// the base; layers above it apply prepend/append/wrap in ascending
// priority order, mirroring common.sh's resolve_template_content.
func composeLayers(layers []layer) (string, error) {
	baseIdx := -1
	for i, l := range layers {
		if l.strategy == StrategyReplace {
			baseIdx = i
			break
		}
	}
	if baseIdx < 0 {
		return "", fmt.Errorf("preset: composing layers but no replace base")
	}

	content := layers[baseIdx].content
	for i := baseIdx - 1; i >= 0; i-- {
		l := layers[i]
		switch l.strategy {
		case StrategyReplace:
			content = l.content
		case StrategyPrepend:
			content = l.content + "\n\n" + content
		case StrategyAppend:
			content = content + "\n\n" + l.content
		case StrategyWrap:
			if !strings.Contains(l.content, "{CORE_TEMPLATE}") {
				return "", fmt.Errorf("preset: wrap strategy layer missing {CORE_TEMPLATE} placeholder")
			}
			content = strings.ReplaceAll(l.content, "{CORE_TEMPLATE}", content)
		default:
			return "", fmt.Errorf("preset: unknown strategy %q", l.strategy)
		}
	}
	return content, nil
}
