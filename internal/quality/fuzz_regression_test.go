package quality

import "testing"

func TestBlockPreservesLiteralScalarFinalNewlines(t *testing.T) {
	for _, test := range []struct{ name, body, want string }{
		{"literal", "payload: |+\n  first\n\n", "first\n\n"},
		{"empty-line", "payload: |+\n\n", "\n"},
		{"folded", "payload: >+\n  first\n  second\n\n", "first second\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got map[string]string
			if err := Block([]byte("```mulix-acceptance\n"+test.body+"```\n"), "mulix-acceptance", &got); err != nil {
				t.Fatal(err)
			}
			if got["payload"] != test.want {
				t.Fatalf("fenced YAML scalar = %q, want %q", got["payload"], test.want)
			}
		})
	}
}
