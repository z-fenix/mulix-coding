// Package assets embeds Mulix resources for offline project setup and updates.
package assets

import "embed"

// Skills embeds the phase skills; setup-mulix is distributed separately for bootstrap.
//
//go:embed skills/mulix-*
var Skills embed.FS

//go:embed templates
var Templates embed.FS

//go:embed presets
var Presets embed.FS

// Superpowers holds every superpowers skill (superpowers/skills/<name>/,
// whole directories), rewritten for mulix by scripts/sync-superpowers.sh,
// plus the upstream LICENSE and a VERSION stamp.
//
//go:embed superpowers
var Superpowers embed.FS
