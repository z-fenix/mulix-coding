package quality

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func FuzzAcceptanceBlock(f *testing.F) {
	for _, seed := range []string{"FR-001", "中文SC-002：验收", "```\n~~~\n\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		// Block promises YAML, not arbitrary JSON's wider raw-control alphabet.
		// Marshal valid YAML independently; choose a fence longer than any payload run.
		encoded, err := yaml.Marshal(map[string]string{"payload": payload})
		if err != nil {
			t.Fatal(err)
		}
		longest, run := 2, 0
		for _, c := range encoded {
			if c == '`' {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		// Compare extraction against the unfenced YAML document, not the encoder's
		// input: yaml.v3 itself canonicalizes some empty block scalars.
		var expected map[string]string
		if err := yaml.Unmarshal(encoded, &expected); err != nil {
			t.Fatal(err)
		}
		marker := strings.Repeat("`", longest+1)
		md := marker + "mulix-acceptance\n" + string(encoded) + marker + "\n"
		var got map[string]string
		if err := Block([]byte(md), "mulix-acceptance", &got); err != nil || got["payload"] != expected["payload"] {
			t.Fatalf("fenced YAML differs from unfenced YAML: %v, got=%q want=%q", err, got["payload"], expected["payload"])
		}
		if err := Block([]byte(md+md), "mulix-acceptance", &got); err == nil {
			t.Fatal("duplicate contract fences were accepted")
		}
		var arbitrary any
		_ = Block([]byte(payload), "mulix-acceptance", &arbitrary)
	})
}
