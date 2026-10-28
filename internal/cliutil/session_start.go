package cliutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/hook"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func newSessionStartCmd() *cobra.Command {
	return &cobra.Command{Use: "session-start", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		cwd, _ := os.Getwd()
		return runSessionStart(cmd.Context(), cwd, os.Getenv("CLAUDE_ENV_FILE"), payload, os.Stdout, os.Stderr)
	}}
}

// runSessionStart never changes selection. A failed binding is guidance for the
// user, not a reason to stop the host starting or secretly select another change.
func runSessionStart(ctx context.Context, processCWD, envFile string, payload []byte, out, diagnostics io.Writer) error {
	req, parseErr := hook.ParseRequest(bytes.NewReader(payload))
	text := "mulix: session context unavailable; run mulix context show to diagnose."
	if parseErr != nil {
		fmt.Fprintf(diagnostics, "mulix session-start: %v\n", parseErr)
	} else {
		id := sessionctx.Identity{}
		if req.SessionID != nil {
			id = sessionctx.Identity{Value: *req.SessionID, Present: true}
		}
		if err := sessionctx.ValidateIdentity(id); err != nil {
			fmt.Fprintf(diagnostics, "mulix session-start: %v\n", err)
		} else {
			text = "mulix: no host session ID supplied; commands use global context."
			if id.Present {
				argument, _ := hook.SessionArgument(id)
				text = "mulix session identity: " + argument + ". Use --session-id " + argument + " on mulix commands when environment injection is unavailable; never infer identity from another session."
				if envFile != "" {
					line, _ := hook.SessionEnvLine(id)
					if err := appendSessionEnv(envFile, line); err != nil {
						fmt.Fprintf(diagnostics, "mulix session environment: %v; use explicit --session-id\n", err)
					}
				}
			}
			start := req.CWD
			if start == "" {
				start = processCWD
			}
			info, dirErr := os.Stat(start)
			if dirErr == nil && !info.IsDir() {
				dirErr = fmt.Errorf("cwd is not a directory")
			}
			if dirErr == nil {
				store, err := sessionctx.Open(start)
				if err == nil {
					r, err := store.Resolve(ctx, sessionctx.ResolveRequest{Identity: id})
					if err != nil {
						text += " Context invalid: " + err.Error() + ". Repair with state select or context unbind for this session; do not fall back."
					} else {
						text += fmt.Sprintf(" Current mode=%s change=%s phase=%s source=%s. Run mulix state show before work; this startup snapshot is not a cached change.", r.Mode, r.Change, r.Phase, r.Source)
					}
				} else {
					fmt.Fprintf(diagnostics, "mulix session context: %v\n", err)
				}
			} else {
				fmt.Fprintf(diagnostics, "mulix session cwd: %v\n", dirErr)
			}
		}
	}
	data, err := hook.RenderSessionStart(text)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
func appendSessionEnv(path, line string) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if _, err = io.WriteString(f, "\n"+line); err != nil {
		return err
	}
	return f.Sync()
}
