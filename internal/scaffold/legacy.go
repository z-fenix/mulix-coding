package scaffold

import "path"

const entrySkillName = "mulix-using"

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

func legacyVendorMatch(rel string, data []byte) bool {
	expected, ok := legacyVendorDigests[managedMapKey(legacyVendorDigests, rel)]
	return ok && contentDigest(data) == expected
}
