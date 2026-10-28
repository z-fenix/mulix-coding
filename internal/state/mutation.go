package state

import (
	"context"
	"fmt"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
)

type UpdateResult struct {
	State flow.State
	Saved bool
}

// Update protects the entire read/check/write operation. afterSave runs under
// the change lock and may take the context lock, but must not reenter this lock.
func Update(ctx context.Context, root, change string, mutate func(*flow.State) error, afterSave func(flow.State) error) (result UpdateResult, err error) {
	err = lockfile.With(ctx, root, lockfile.ChangeKey(change), func() error {
		current, err := Load(root, change)
		if err != nil {
			return err
		}
		if current.Change != change {
			return fmt.Errorf("state identity %q does not match locked change %q", current.Change, change)
		}
		schema := current.Schema
		if err := mutate(&current); err != nil {
			return err
		}
		if current.Change != change || current.Schema != schema {
			return fmt.Errorf("state update must not change schema or change identity")
		}
		if err := Save(root, current); err != nil {
			return err
		}
		result.State, result.Saved = current, true
		if afterSave != nil {
			return afterSave(current)
		}
		return nil
	})
	return
}
