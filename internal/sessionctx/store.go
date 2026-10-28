package sessionctx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/state"
)

type Store struct {
	root      string
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, fs.FileMode) error
}
type config struct {
	Schema string `json:"schema"`
	Mode   Mode   `json:"mode"`
}
type binding struct {
	Schema    string    `json:"schema"`
	SessionID string    `json:"session_id"`
	Change    string    `json:"change"`
	BoundAt   time.Time `json:"bound_at"`
}

func Open(start string) (*Store, error) {
	root, err := state.FindRoot(start)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, readFile: os.ReadFile, writeFile: fileutil.WriteFile}, nil
}
func (s *Store) Root() string { return s.root }
func bindingRel(id string) string {
	return fmt.Sprintf("%s/%x.json", layout.SessionsDir, sha256.Sum256([]byte(id)))
}
func (s *Store) read(rel string) ([]byte, error) {
	p, err := fileutil.ControlPath(s.root, rel, false)
	if err != nil {
		return nil, err
	}
	return s.readFile(p)
}
func strictJSON(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected one JSON value, got %v", err)
	}
	return nil
}
func (s *Store) writeJSON(rel string, value any) error {
	p, err := fileutil.ControlPath(s.root, rel, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.writeFile(p, append(data, '\n'), 0600)
}
func (s *Store) modeUnlocked() (Mode, error) {
	data, err := s.read(layout.ContextFile)
	if errors.Is(err, fs.ErrNotExist) {
		return Legacy, nil
	}
	var c config
	if err == nil {
		err = strictJSON(data, &c)
	}
	if err == nil && (c.Schema != "mulix.context.v1" || (c.Mode != Legacy && c.Mode != Session)) {
		err = fmt.Errorf("invalid schema or mode")
	}
	if err != nil {
		return "", &ResolveError{Kind: "config-invalid", Path: layout.ContextFile, Err: err}
	}
	return c.Mode, nil
}

// ModeHint reads only the strictly decoded policy file, without acquiring the
// context lock. Hooks use it solely to classify an acquisition failure: missing
// or readable legacy configuration can fail open; invalid/unreadable policy
// cannot. It is not a selection snapshot and must not resolve changes.
func (s *Store) ModeHint() (Mode, error) { return s.modeUnlocked() }

func (s *Store) Mode(ctx context.Context) (mode Mode, err error) {
	err = lockfile.With(ctx, s.root, lockfile.ContextKey, func() error { var e error; mode, e = s.modeUnlocked(); return e })
	return
}
func (s *Store) SetMode(ctx context.Context, mode Mode) error {
	if mode != Legacy && mode != Session {
		return fmt.Errorf("mode must be legacy or session")
	}
	return lockfile.With(ctx, s.root, lockfile.ContextKey, func() error { return s.writeJSON(layout.ContextFile, config{Schema: "mulix.context.v1", Mode: mode}) })
}
