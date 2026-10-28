package cliutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/z-fenix/mulix-coding/internal/hook"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

type hookOutcome struct {
	Decision hook.Decision
	Resolved sessionctx.Resolved
	Cause    error
}

func contextFailure(r sessionctx.Resolved, cause error) hookOutcome {
	allow := r.Root == "" || r.Mode == sessionctx.Legacy
	return hookOutcome{Resolved: r, Cause: cause, Decision: hook.Decision{
		Allow:  allow,
		Reason: fmt.Sprintf("change=%s phase=%s source=%s: %v", r.Change, r.Phase, r.Source, cause),
		Next:   "Repair the context with mulix context mode, state select --session-id <id>, or context unbind --session-id <id>; an invalid binding never falls back.",
	}}
}

func malformedHook(ctx context.Context, cwd string, cause error) hookOutcome {
	s, err := sessionctx.Open(cwd)
	if err != nil {
		return contextFailure(sessionctx.Resolved{}, cause)
	}
	mode, modeErr := s.Mode(ctx)
	if modeErr != nil {
		// A broken lock must not prevent confirming an opt-out/legacy policy.
		// Only the policy file is inspected; no change selection occurs here.
		var hintErr error
		mode, hintErr = s.ModeHint()
		cause = errors.Join(cause, modeErr, hintErr)
	}
	return contextFailure(sessionctx.Resolved{Root: s.Root(), Mode: mode, Source: sessionctx.Global}, cause)
}

func evaluatePreToolUse(ctx context.Context, processCWD string, payload []byte) hookOutcome {
	req, err := hook.ParseRequest(bytes.NewReader(payload))
	if err != nil {
		return malformedHook(ctx, processCWD, err)
	}
	if (req.ToolName != "Write" && req.ToolName != "Edit") || req.ToolInput.FilePath == "" {
		return hookOutcome{Decision: hook.Decision{Allow: true}}
	}
	start := req.CWD
	if start == "" {
		start = processCWD
	}
	info, err := os.Stat(start)
	if err != nil {
		return contextFailure(sessionctx.Resolved{}, fmt.Errorf("hook cwd %q: %w", start, err))
	}
	if !info.IsDir() {
		return contextFailure(sessionctx.Resolved{}, fmt.Errorf("hook cwd %q is not a directory", start))
	}
	store, err := sessionctx.Open(start)
	if err != nil {
		return contextFailure(sessionctx.Resolved{}, err)
	}
	id := sessionctx.Identity{}
	if req.SessionID != nil {
		id = sessionctx.Identity{Value: *req.SessionID, Present: true}
	}
	r, err := store.Resolve(ctx, sessionctx.ResolveRequest{Identity: id, Access: sessionctx.Inspect})
	if err != nil {
		// Identity can fail before mode resolution. An unknown managed policy
		// must not silently downgrade; a confirmed legacy policy still opens.
		if r.Mode == "" {
			mode, hintErr := store.ModeHint()
			if hintErr == nil {
				r.Mode = mode
			} else {
				err = errors.Join(err, hintErr)
			}
		}
		return contextFailure(r, err)
	}
	return hookOutcome{Resolved: r, Decision: hook.DecideContext(store.Root(), r, req)}
}

func emitHook(out io.Writer, decision hook.Decision, strict bool, render func(hook.Decision) ([]byte, error)) int {
	data, err := render(decision)
	if err == nil {
		_, err = fmt.Fprintln(out, string(data))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "mulix hook output: %v\n", err)
		if strict {
			return 2
		}
		return 0
	}
	return hook.ExitCode(decision)
}
