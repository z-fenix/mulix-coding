package sessionctx

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func testID(value string) Identity { return Identity{Value: value, Present: true} }
func contextFixture(t *testing.T) (string, *Store) {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"X", "Y"} {
		s := flow.New(id, "")
		s.Phase = flow.PhaseBuild
		if err := state.Save(root, s); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(context.Background(), Session); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(root, "X"); err != nil {
		t.Fatal(err)
	}
	return s.Root(), s
}
func bindTest(t *testing.T, s *Store, id, change string) {
	t.Helper()
	target, err := s.PrepareSelection(context.Background(), testID(id), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(context.Background(), target, change); err != nil {
		t.Fatal(err)
	}
}
func bindingFile(root, id string) string {
	return filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte(id))))
}
func resolveTest(s *Store, id string) (Resolved, error) {
	return s.Resolve(context.Background(), ResolveRequest{Identity: testID(id)})
}

func TestResolve_BoundAndUnboundAreIndependent(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "X")
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	a, err := resolveTest(s, "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolveTest(s, "B")
	if err != nil {
		t.Fatal(err)
	}
	if a.Change != "X" || a.Source != SessionBinding || b.Change != "Y" || b.Source != GlobalFallback {
		t.Fatalf("crossed: A=%+v B=%+v", a, b)
	}
	if _, err := os.Lstat(bindingFile(root, "B")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("fallback created binding: %v", err)
	}
	if !filepath.IsAbs(b.RuntimeDir) {
		t.Fatal("runtime directory is not absolute")
	}
	if err := state.ClearActive(root); err != nil {
		t.Fatal(err)
	}
	b, err = resolveTest(s, "B")
	if err != nil || b.Change != "" || b.State != nil || !strings.HasSuffix(filepath.ToSlash(b.RuntimeDir), "/.mulix/.runtime/_shared") {
		t.Fatalf("empty context: %+v %v", b, err)
	}
}

func TestResolve_InvalidBindingNeverFallsBack(t *testing.T) {
	for _, bad := range []string{"{", "null", `{"schema":"unknown"}`, `{"schema":"mulix.session.v1","session_id":"other","change":"X","bound_at":"2026-10-28T00:00:00Z"}`, `{"schema":"mulix.session.v1","session_id":"A","change":"missing","bound_at":"2026-10-28T00:00:00Z"}`, `{"schema":"mulix.session.v1","session_id":"A","change":"X","bound_at":"2026-10-28T00:00:00Z","extra":1}`} {
		t.Run(bad, func(t *testing.T) {
			root, s := contextFixture(t)
			bindTest(t, s, "A", "X")
			if err := os.WriteFile(bindingFile(root, "A"), []byte(bad), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := resolveTest(s, "A")
			if err == nil || got.Source != SessionBinding {
				t.Fatalf("invalid binding fell back: %+v %v", got, err)
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		root, s := contextFixture(t)
		bindTest(t, s, "A", "X")
		p := bindingFile(root, "A")
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveTest(s, "A"); err == nil {
			t.Fatal("directory binding fell back")
		}
		if _, err := s.Unbind(context.Background(), testID("A")); err == nil {
			t.Fatal("unbind removed directory")
		}
	})
}

func TestResolve_ArchivePhaseIsNotArchived(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "X")
	x, _ := state.Load(root, "X")
	x.Phase = flow.PhaseArchive
	if err := state.Save(root, x); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTest(s, "A"); err != nil {
		t.Fatal("archive phase is not completed archive:", err)
	}
	x.Archived = true
	if err := state.Save(root, x); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTest(s, "A"); err == nil {
		t.Fatal("archived binding accepted")
	}
	if got, err := s.Resolve(context.Background(), ResolveRequest{Identity: testID("A"), ExplicitChange: "X", Access: Inspect}); err != nil || got.Source != ExplicitChange {
		t.Fatalf("diagnostic query: %+v %v", got, err)
	}
}

func TestResolve_StateMismatchAndErrorsAreNotFallback(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "X")
	x, _ := state.Load(root, "X")
	x.Change = "Y"
	data, err := os.ReadFile(state.PathFor(root, "X"))
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "change: X", "change: Y", 1))
	if err := os.WriteFile(state.PathFor(root, "X"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTest(s, "A"); err == nil {
		t.Fatal("state identity mismatch accepted")
	}
	if err := os.Remove(state.PathFor(root, "X")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTest(s, "A"); err == nil {
		t.Fatal("missing state fell back")
	}
}

func TestResolve_LegacyIgnoresBindingsAndModeRoundTripPreservesThem(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "Y")
	before, _ := os.ReadFile(bindingFile(root, "A"))
	if err := s.SetMode(context.Background(), Legacy); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTest(s, "A")
	if err != nil || got.Change != "X" || got.Source != Global {
		t.Fatalf("legacy: %+v %v", got, err)
	}
	if err := s.SetMode(context.Background(), Session); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(bindingFile(root, "A"))
	if string(before) != string(after) {
		t.Fatal("mode change edited binding")
	}
	got, err = resolveTest(s, "A")
	if err != nil || got.Change != "Y" {
		t.Fatalf("restored: %+v %v", got, err)
	}
	if err := os.Remove(filepath.Join(root, ".mulix", "context.json")); err != nil {
		t.Fatal(err)
	}
	got, err = resolveTest(s, "A")
	if err != nil || got.Mode != Legacy || got.Change != "X" {
		t.Fatalf("missing config: %+v %v", got, err)
	}
}

func TestResolve_ConfigAndActiveErrorsAreNotMissing(t *testing.T) {
	for _, bad := range []string{"", "null", `{}`, `{"schema":"mulix.context.v1","mode":"session","extra":true}`, `{"schema":"mulix.context.v1","mode":"session"} {}`} {
		t.Run("config:"+bad, func(t *testing.T) {
			root, s := contextFixture(t)
			if err := os.WriteFile(filepath.Join(root, ".mulix", "context.json"), []byte(bad), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := resolveTest(s, "B"); err == nil {
				t.Fatal("invalid configuration downgraded")
			}
			if err := s.SetMode(context.Background(), Session); err != nil {
				t.Fatal("explicit repair:", err)
			}
		})
	}
	for _, active := range []string{"", "missing", "../outside", "_shared"} {
		t.Run("active:"+active, func(t *testing.T) {
			root, s := contextFixture(t)
			if err := os.WriteFile(filepath.Join(root, ".mulix", "active"), []byte(active), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := resolveTest(s, "B"); err == nil {
				t.Fatal("invalid active treated as absent")
			}
		})
	}
}

func TestResolve_ExplicitChangeCannotBypassMutationContext(t *testing.T) {
	_, s := contextFixture(t)
	for _, id := range []string{"A", "B"} {
		if id == "A" {
			bindTest(t, s, id, "X")
		}
		_, err := s.Resolve(context.Background(), ResolveRequest{Identity: testID(id), ExplicitChange: "Y", Access: Mutate})
		if err == nil {
			t.Fatalf("%s bypassed current change", id)
		}
		if _, err := s.Resolve(context.Background(), ResolveRequest{Identity: testID(id), ExplicitChange: "Y", Access: Inspect}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Resolve(context.Background(), ResolveRequest{ExplicitChange: "Y", Access: Mutate}); err != nil {
		t.Fatal("manual explicit management:", err)
	}
}

func TestSelect_UnbindAndFailureRecovery(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "Y")
	old, _ := os.ReadFile(bindingFile(root, "A"))
	originalWrite := s.writeFile
	s.writeFile = func(string, []byte, fs.FileMode) error { return fs.ErrPermission }
	target, _ := s.PrepareSelection(context.Background(), testID("A"), false)
	if _, err := s.Select(context.Background(), target, "X"); err == nil {
		t.Fatal("ignored replacement failure")
	}
	still, _ := os.ReadFile(bindingFile(root, "A"))
	if string(old) != string(still) {
		t.Fatal("failed replacement destroyed binding")
	}
	s.writeFile = originalWrite
	if err := os.WriteFile(bindingFile(root, "A"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	bindTest(t, s, "A", "Y")
	if removed, err := s.Unbind(context.Background(), testID("A")); err != nil || !removed {
		t.Fatalf("unbind: %v %v", removed, err)
	}
	got, err := resolveTest(s, "A")
	if err != nil || got.Change != "X" || got.Source != GlobalFallback {
		t.Fatalf("fallback after unbind: %+v %v", got, err)
	}
	if removed, err := s.ClearActiveIf(context.Background(), "Y"); err != nil || removed {
		t.Fatalf("cleared other active: %v %v", removed, err)
	}
}

func TestIdentityAndChangeValidation(t *testing.T) {
	for _, id := range []string{"", " ", " A", "A ", "A\nB", string([]byte{255}), strings.Repeat("a", 513)} {
		if ValidateIdentity(testID(id)) == nil {
			t.Fatalf("accepted identity %q", id)
		}
	}
	for _, id := range []string{"A", "a'b c", strings.Repeat("é", 256)} {
		if err := ValidateIdentity(testID(id)); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range []string{"", ".", "..", "../a", `a\b`, "C:a", "_shared", "CON", "con.txt", "LPT1", "x.", "x ", "a/b"} {
		if ValidateChange(change) == nil {
			t.Fatalf("accepted change %q", change)
		}
	}
	for _, change := range []string{"001-old", "2026-10-28-login", "add-login", "X"} {
		if err := ValidateChange(change); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolve_ReadErrorDoesNotFallBack(t *testing.T) {
	root, s := contextFixture(t)
	bindTest(t, s, "A", "X")
	read := s.readFile
	s.readFile = func(path string) ([]byte, error) {
		if path == bindingFile(root, "A") {
			return nil, fs.ErrPermission
		}
		return read(path)
	}
	if _, err := resolveTest(s, "A"); err == nil {
		t.Fatal("permission error fell back")
	}
}

func TestResolve_ProjectAndNestedRootIsolation(t *testing.T) {
	root, first := contextFixture(t)
	bindTest(t, first, "A", "X")
	_, second := contextFixture(t)
	bindTest(t, second, "A", "Y")
	got, err := resolveTest(second, "A")
	if err != nil || got.Change != "Y" {
		t.Fatalf("second root: %+v %v", got, err)
	}
	nested := filepath.Join(root, "nested")
	if err := state.Save(nested, flow.New("nested", "")); err != nil {
		t.Fatal(err)
	}
	n, err := Open(filepath.Join(nested, ".mulix"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Root() != nested {
		t.Fatalf("used outer root: %s", n.Root())
	}
}
