package cliutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func TestHookEval_LegacyLockFailureStillFailsOpen(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		allow        bool
	}{
		{"missing", "", true},
		{"legacy", `{"schema":"mulix.context.v1","mode":"legacy"}`, true},
		{"session", `{"schema":"mulix.context.v1","mode":"session"}`, false},
		{"invalid", "{", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".mulix", ".locks", "context.lock"), 0700); err != nil {
				t.Fatal(err)
			}
			if tc.config != "" {
				if err := os.WriteFile(filepath.Join(root, ".mulix", "context.json"), []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, payload := range [][]byte{hookPayload(t, root, "A", "Write", "src/code.go"), []byte("{")} {
				out := evaluatePreToolUse(context.Background(), root, payload)
				if out.Cause == nil || out.Decision.Allow != tc.allow {
					t.Fatalf("policy=%s allow=%v: %+v", tc.name, tc.allow, out)
				}
			}
		})
	}
}

func TestHookEval_ConfirmedLegacyTimeoutFailsOpen(t *testing.T) {
	root, s := sessionProject(t)
	if err := s.SetMode(context.Background(), sessionctx.Legacy); err != nil {
		t.Fatal(err)
	}
	ready, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- lockfile.With(context.Background(), root, lockfile.ContextKey, func() error { close(ready); <-release; return nil })
	}()
	<-ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	out := evaluatePreToolUse(ctx, root, hookPayload(t, root, "A", "Write", "src/code.go"))
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !out.Decision.Allow || out.Cause == nil || out.Resolved.Mode != sessionctx.Legacy {
		t.Fatalf("legacy timeout became strict: %+v", out)
	}
}
