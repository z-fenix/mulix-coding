package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/state"
)

func TestStatus_BoundSessionStillReportsInvalidGlobalContext(t *testing.T) {
	for _, target := range []string{"missing-target", "bad/path", "", "archived"} {
		t.Run(target, func(t *testing.T) {
			root, s := sessionProject(t)
			selectForSession(t, s, "A", "X")
			if target == "archived" {
				y, _ := state.Load(root, "Y")
				y.Archived = true
				saveStateForTest(t, root, y)
				if err := state.SetActive(root, "Y"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(filepath.Join(root, ".mulix", "active"), []byte(target+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runPresetCLI(t, "status", "--session-id", "A")
			if err == nil || !strings.Contains(out, "global context error:") || !strings.Contains(out, "session-binding") || !strings.Contains(out, "X") {
				t.Fatalf("bad global hidden by valid binding: %q %v", out, err)
			}
		})
	}
}
