package sessionctx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func (s *Store) Resolve(ctx context.Context, req ResolveRequest) (Resolved, error) {
	r := Resolved{Root: s.root, Source: Global}
	if req.Identity.Present {
		r.SessionID = req.Identity.Value
	}
	if err := ValidateIdentity(req.Identity); err != nil {
		return r, err
	}
	closingArchive := req.Access == CloseArchivedSpec
	if closingArchive && req.ExplicitChange == "" {
		return r, fmt.Errorf("closing an archived spec requires an explicit --change")
	}
	mutating := req.Access == Mutate || closingArchive
	// Snapshot pointers under the context lock; never wait for a change while
	// holding it. A subsequent global switch only affects the next invocation.
	// A strictly read-only preview instead sees atomic file snapshots and may
	// race a selection change; the writer must independently resolve again.
	read := func(key string, fn func() error) error {
		if req.Access == InspectReadOnly {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fn()
		}
		return lockfile.With(ctx, s.root, key, fn)
	}
	err := read(lockfile.ContextKey, func() error {
		var err error
		r.Mode, err = s.modeUnlocked()
		if err != nil {
			return err
		}
		if req.ExplicitChange != "" && (!mutating || r.Mode == Legacy || !req.Identity.Present) {
			r.Change, r.Source = req.ExplicitChange, ExplicitChange
			return nil
		}
		if r.Mode == Session && req.Identity.Present {
			r.Source = SessionBinding
			data, err := s.read(bindingRel(req.Identity.Value))
			if !errors.Is(err, fs.ErrNotExist) {
				var b binding
				if err == nil {
					err = strictJSON(data, &b)
				}
				if err == nil && (b.Schema != "mulix.session.v1" || b.SessionID != req.Identity.Value || b.BoundAt.IsZero()) {
					err = fmt.Errorf("invalid schema, identity or binding timestamp")
				}
				if err == nil {
					r.Change = b.Change
					err = ValidateChange(b.Change)
				}
				if err != nil {
					return &ResolveError{Kind: "binding-invalid", Path: bindingRel(req.Identity.Value), Err: err}
				}
				return nil
			}
			r.Source = GlobalFallback
		}
		data, err := s.read(".mulix/active")
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err == nil {
			r.Change = strings.TrimSpace(string(data))
			err = ValidateChange(r.Change)
		}
		if err != nil {
			return &ResolveError{Kind: "active-invalid", Path: ".mulix/active", Err: err}
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	if closingArchive && r.Change == "" {
		// Archive normally clears global active. An unbound session may
		// explicitly name the archived record in that case.
		r.Change, r.Source = req.ExplicitChange, ExplicitChange
	}
	if r.Mode == Session && req.Identity.Present && mutating && req.ExplicitChange != "" && req.ExplicitChange != r.Change {
		return r, &ResolveError{Kind: "binding-invalid", Err: fmt.Errorf("--change %q differs from current change %q; select it first", req.ExplicitChange, r.Change)}
	}
	if r.Change == "" {
		r.ScratchDir, err = scratchDir(s.root, r.Change, r.SessionID)
		return r, err
	}
	if err := ValidateChange(r.Change); err != nil {
		return r, &ResolveError{Kind: "state-invalid", Err: err}
	}
	r.RuntimeDir = filepath.Join(s.root, filepath.FromSlash(layout.ChangeRuntimeDir(r.Change)))
	err = read(lockfile.ChangeKey(r.Change), func() error {
		if _, err := fileutil.ControlPath(s.root, layout.ChangeRuntimePath(r.Change, layout.StateFile), false); err != nil {
			return err
		}
		loaded, err := state.Load(s.root, r.Change)
		if err != nil {
			return err
		}
		if r.Mode == Session && loaded.Change != r.Change {
			return fmt.Errorf("state change %q does not match %q", loaded.Change, r.Change)
		}
		if closingArchive && !loaded.Archived {
			return fmt.Errorf("spec Issue closure requires an archived change")
		}
		if r.Mode == Session && loaded.Archived && r.Source != ExplicitChange && !closingArchive {
			return fmt.Errorf("change %q is archived; select another change or unbind", r.Change)
		}
		r.State, r.Phase = &loaded, loaded.Phase
		return nil
	})
	if err != nil {
		return r, &ResolveError{Kind: "state-invalid", Path: state.PathFor(s.root, r.Change), Err: err}
	}
	r.ScratchDir, err = scratchDir(s.root, r.Change, r.SessionID)
	return r, err
}
