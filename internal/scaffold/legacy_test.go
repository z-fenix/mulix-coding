package scaffold

import (
	"bytes"
	"os"
	"testing"
)

func TestLegacyVendor_ExactRenderedOfficialBytesOnly(t *testing.T) {
	current, err := os.ReadFile("testdata/legacy/using-mulix.md")
	if err != nil {
		t.Fatal(err)
	}
	historical, err := os.ReadFile("testdata/legacy/mulix-using-mulix.md")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rel  string
		data []byte
	}{
		{".claude/skills/using-mulix/SKILL.md", bytes.ReplaceAll(current, []byte("{{SKILLS_DIR}}"), []byte(".claude/skills"))},
		{".dsh/skills/using-mulix/SKILL.md", bytes.ReplaceAll(current, []byte("{{SKILLS_DIR}}"), []byte(".dsh/skills"))},
		{".claude/skills/mulix-using-mulix/SKILL.md", historical},
	}
	for _, c := range cases {
		if !legacyVendorMatch(c.rel, c.data) {
			t.Fatalf("known official snapshot not recognized: %s", c.rel)
		}
		edited := append(append([]byte{}, c.data...), []byte("\nLOCAL\n")...)
		if legacyVendorMatch(c.rel, edited) {
			t.Fatalf("local customization treated as official: %s", c.rel)
		}
		if legacyVendorMatch(".claude/skills/unrelated/SKILL.md", c.data) {
			t.Fatal("fingerprint accepted for unrelated path")
		}
	}
}
