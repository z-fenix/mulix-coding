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
