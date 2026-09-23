package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/scaffold"
)

func newInitCmd() *cobra.Command {
	var force bool
	var dir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install mulix into a project: .mulix/ state dir, Claude Code skills, and the PreToolUse hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := scaffold.Init(scaffold.InitOptions{Root: dir, Force: force})
			if err != nil {
				return err
			}
			for _, w := range res.Written {
				fmt.Printf("  wrote   %s\n", w)
			}
			for _, s := range res.Skipped {
				fmt.Printf("  skipped %s (already present; use --force to overwrite)\n", s)
			}
			fmt.Println("mulix initialized. Run `mulix new \"<title>\"` to start your first change.")
			if found, _ := scaffold.DetectSuperpowers(dir); !found {
				fmt.Println("hint: the superpowers plugin wasn't detected. The build phase can delegate task execution to its writing-plans/subagent-driven-development/test-driven-development skill chain when it's installed — see the mulix-build skill for details.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files mulix already installed")
	cmd.Flags().StringVar(&dir, "dir", ".", "target project directory")
	return cmd
}
