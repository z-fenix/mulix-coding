package sessionctx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func (s *Store) PrepareSelection(ctx context.Context, id Identity, global bool) (SelectionTarget, error) {
	target := SelectionTarget{Identity: id, Global: global}
	if err := ValidateIdentity(id); err != nil {
		return target, err
	}
	mode, err := s.Mode(ctx)
	target.Mode = mode
	return target, err
}

func (s *Store) Select(ctx context.Context, target SelectionTarget, change string) (Selection, error) {
	result := Selection{Mode: target.Mode, Change: change, Scope: "global"}
	if target.Identity.Present {
		result.SessionID = target.Identity.Value
	}
	if err := ValidateIdentity(target.Identity); err != nil {
		return result, err
	}
	if err := ValidateChange(change); err != nil {
		return result, err
	}
	if target.Mode != Legacy && target.Mode != Session {
		return result, fmt.Errorf("invalid selection mode")
	}
	if target.Mode == Session && target.Identity.Present && !target.Global {
		result.Scope = "session"
	}
	err := lockfile.With(ctx, s.root, lockfile.ChangeKey(change), func() error {
		if _, err := fileutil.ControlPath(s.root, ".mulix/.runtime/"+change+"/state.yaml", false); err != nil {
			return err
		}
		loaded, err := state.Load(s.root, change)
		if err != nil {
			return err
		}
		if loaded.Change != change || loaded.Archived {
			return fmt.Errorf("change %q has mismatched or archived state", change)
		}
		return lockfile.With(ctx, s.root, lockfile.ContextKey, func() error {
			if result.Scope == "session" {
				return s.writeJSON(bindingRel(target.Identity.Value), binding{Schema: "mulix.session.v1", SessionID: target.Identity.Value, Change: change, BoundAt: time.Now().UTC()})
			}
			p, err := fileutil.ControlPath(s.root, ".mulix/active", true)
			if err != nil {
				return err
			}
			return s.writeFile(p, []byte(change+"\n"), 0600)
		})
	})
	return result, err
}

func (s *Store) Unbind(ctx context.Context, id Identity) (removed bool, err error) {
	if err := ValidateIdentity(id); err != nil {
		return false, err
	}
	if !id.Present {
		return false, fmt.Errorf("unbind requires --session-id or MULIX_SESSION_ID")
	}
	err = lockfile.With(ctx, s.root, lockfile.ContextKey, func() error {
		p, err := fileutil.ControlPath(s.root, bindingRel(id.Value), false)
		if err != nil {
			return err
		}
		err = os.Remove(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		removed = err == nil
		return err
	})
	return
}

// ClearActiveIf must be called after the state save, still under its change
// lock. A concurrent selection of a different change must not be removed.
func (s *Store) ClearActiveIf(ctx context.Context, change string) (cleared bool, err error) {
	err = lockfile.With(ctx, s.root, lockfile.ContextKey, func() error {
		data, err := s.read(".mulix/active")
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(data)) != change {
			return nil
		}
		p, err := fileutil.ControlPath(s.root, ".mulix/active", false)
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		cleared = true
		return nil
	})
	return
}
