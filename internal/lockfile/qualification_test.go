package lockfile

import (
	"context"
	"errors"
	"testing"
)

func TestLock_PreCancelledNeverRunsAndCanRetry(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	err := With(ctx, root, ContextKey, func() error { ran = true; return nil })
	if ran || !errors.Is(err, ErrTimeout) || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled callback ran=%v, error=%v", ran, err)
	}
	if err := With(context.Background(), root, ContextKey, func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("retry after pre-cancellation: ran=%v, error=%v", ran, err)
	}
}
