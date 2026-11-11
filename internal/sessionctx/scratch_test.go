package sessionctx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/state"
)

// Read the public JSON boundary so missing fields fail as behavior, not compilation.
func serializedScratch(t *testing.T, r Resolved) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	return fields["scratch_dir"]
}

func TestScratchDir_IsStableAndContextScoped(t *testing.T) {
	temp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, temp)
	}
	_, s := contextFixture(t)
	bindTest(t, s, "A", "X")
	bindTest(t, s, "B", "X")
	a, err := resolveTest(s, "A")
	if err != nil {
		t.Fatal(err)
	}
	first := serializedScratch(t, a)
	if !filepath.IsAbs(first) || filepath.Dir(first) != filepath.Clean(os.TempDir()) || !regexp.MustCompile(`^mulix-scratch-[0-9a-f]{64}$`).MatchString(filepath.Base(first)) {
		t.Fatalf("not a context-scoped temporary path: %q", first)
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("resolving context created scratch files: %v", err)
	}
	again, err := resolveTest(s, "A")
	if err != nil || serializedScratch(t, again) != first {
		t.Fatalf("same context is not stable: %+v %v", again, err)
	}
	b, err := resolveTest(s, "B")
	if err != nil {
		t.Fatal(err)
	}
	otherChange, err := s.Resolve(context.Background(), ResolveRequest{Identity: testID("A"), ExplicitChange: "Y", Access: Inspect})
	if err != nil {
		t.Fatal(err)
	}
	_, other := contextFixture(t)
	bindTest(t, other, "A", "X")
	otherRoot, err := resolveTest(other, "A")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{first: true}
	for _, r := range []Resolved{b, otherChange, otherRoot} {
		p := serializedScratch(t, r)
		if p == "" || seen[p] {
			t.Fatalf("contexts share scratch: %+v -> %q", r, p)
		}
		seen[p] = true
	}
}

func TestScratchDir_EmptyContextIsValidButErrorsHaveNoScratch(t *testing.T) {
	root, s := contextFixture(t)
	if err := state.ClearActive(root); err != nil {
		t.Fatal(err)
	}
	r, err := resolveTest(s, "A")
	if err != nil || r.RuntimeDir != "" || r.State != nil || !filepath.IsAbs(serializedScratch(t, r)) {
		t.Fatalf("empty context must have scratch, not runtime: %+v %v", r, err)
	}
	bindTest(t, s, "A", "X")
	if err := os.WriteFile(bindingFile(root, "A"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = resolveTest(s, "A")
	if err == nil || serializedScratch(t, r) != "" {
		t.Fatalf("invalid binding produced a scratch fallback: %+v %v", r, err)
	}
}

func TestScratchDir_WindowsRootCaseIsCanonical(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path identity")
	}
	root, s := contextFixture(t)
	a, err := resolveTest(s, "A")
	if err != nil {
		t.Fatal(err)
	}
	alias, err := Open(strings.ToUpper(root))
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolveTest(alias, "A")
	if err != nil || serializedScratch(t, a) != serializedScratch(t, b) {
		t.Fatalf("root aliases split scratch: %+v %+v %v", a, b, err)
	}
}

func TestScratchDir_RelativeTempRootIsRejected(t *testing.T) {
	root, store := contextFixture(t)
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, "relative temporary files")
	}
	if filepath.IsAbs(os.TempDir()) {
		t.Skip("host expands relative temporary roots")
	}
	for _, noChange := range []bool{false, true} {
		if noChange {
			if err := state.ClearActive(root); err != nil {
				t.Fatal(err)
			}
		}
		r, err := resolveTest(store, "A")
		if err == nil || serializedScratch(t, r) != "" {
			t.Errorf("relative temp root exposed a scratch path (no-change=%t): %+v %v", noChange, r, err)
		}
	}
}
