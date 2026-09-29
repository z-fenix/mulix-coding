// Package assets embeds mulix's bundled skills and templates into the
// compiled binary, so `mulix init` works offline (mirroring spec-kit's
// wheel force-include approach).
package assets

import "embed"

//go:embed skills
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
