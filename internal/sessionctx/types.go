// Package sessionctx resolves per-session changes without changing phase ownership.
package sessionctx

import (
	"fmt"
	"github.com/z-fenix/mulix-coding/internal/flow"
)

type Mode string

const (
	Legacy  Mode = "legacy"
	Session Mode = "session"
)

type Source string

const (
	SessionBinding Source = "session-binding"
	GlobalFallback Source = "global-fallback"
	Global         Source = "global"
	ExplicitChange Source = "explicit-change"
)

type Access int

const (
	Inspect Access = iota
	Mutate
)

type Identity struct {
	Value   string
	Present bool
}
type ResolveRequest struct {
	Identity       Identity
	ExplicitChange string
	Access         Access
}
type Resolved struct {
	Root       string      `json:"root"`
	Mode       Mode        `json:"mode"`
	SessionID  string      `json:"session_id"`
	Change     string      `json:"change"`
	Source     Source      `json:"source"`
	Phase      flow.Phase  `json:"phase"`
	RuntimeDir string      `json:"runtime_dir"`
	State      *flow.State `json:"-"`
}
type SelectionTarget struct {
	Mode     Mode
	Identity Identity
	Global   bool
}
type Selection struct {
	Mode      Mode
	SessionID string
	Change    string
	Scope     string
}
type ResolveError struct {
	Kind string
	Path string
	Err  error
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("context %s (%s): %v", e.Kind, e.Path, e.Err)
}
func (e *ResolveError) Unwrap() error { return e.Err }
