package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/z-fenix/mulix-coding/internal/scaffold"
)

func newInitCmd() *cobra.Command {
	var force bool
	var dir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install mulix into a project: .mulix/, the mulix and superpowers Claude Code skills, and the PreToolUse hook",
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
			fmt.Println("note: the superpowers skills are installed as project skills in .claude/skills/. If the superpowers plugin is also enabled, disable it for this project so its SessionStart bootstrap and namespaced skills don't compete with mulix's phase gates.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files mulix already installed")
	cmd.Flags().StringVar(&dir, "dir", ".", "target project directory")
	return cmd
}
