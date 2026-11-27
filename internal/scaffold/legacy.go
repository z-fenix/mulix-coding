package scaffold

import (
	"path"
	"strings"
)

const entrySkillName = "mulix-using"
const retiredExporterSkill = "mulix-taskstoissues"

var entryAliases = []string{"using-mulix", "mulix-using-mulix"}

func entryRel(host Host, name string) string {
	return path.Join(host.skillsDir(), name, "SKILL.md")
}

// Exact snapshots frozen before this rename, at db38da332abd597ec125df6d63952a0527778d82.
// The first two are assets/skills/using-mulix with its host placeholder rendered.
// The third is the verified official snapshot in examples/todo-cli (left unchanged).
// Other versions, line endings and locally edited snapshots are not guessed.
var legacyVendorDigests = map[string]string{
	".claude/skills/using-mulix/SKILL.md":       "506b636fff71ad1ddd13b98c51a9a6e54016bff04cde17f866c678fe098f8b3c",
	".dsh/skills/using-mulix/SKILL.md":          "08ef0a159bd11f6bbd0caeaad50d9b44eac941b80a73890894ca6f609a006268",
	".claude/skills/mulix-using-mulix/SKILL.md": "440e312e20ab151b578284bf7953ce58de1a1c1cd41a1f71f8f81f0a47781590",
}

// retiredExporterPath classifies case variants for protection on every host.
// The name alone proves neither ownership nor permission to remove bytes.
func retiredExporterPath(rel string) string {
	for _, host := range allHosts {
		canonical := entryRel(host, retiredExporterSkill)
		if strings.EqualFold(rel, canonical) {
			return canonical
		}
	}
	return ""
}

func legacyVendorMatch(rel string, data []byte) bool {
	if retiredExporterPath(rel) != "" {
		// Audited exact LF Git blobs and CRLF copies of the last embedded
		// exporter and frozen todo-cli example. No normalization on removal.
		switch contentDigest(data) {
		case "a72b6a4347c420d23018018ba361ccaa7c4d5095fe0f91c100dcc659eadfbfcb",
			"ce22ce6beeecd3fcfcd5c45247362050eb3b9fc936cc6fc951405e90f8ae4e70",
			"50c120f060fc8df24aa9ffc7d149daa0eedffc27e325fb466475059414c0723f",
			"a2dfcb223f47e5dff34a9675b0c0b63617fb820f8d5bfd93cc011395a519961a":
			return true
		}
		return false
	}
	expected, ok := legacyVendorDigests[managedMapKey(legacyVendorDigests, rel)]
	return ok && contentDigest(data) == expected
}
