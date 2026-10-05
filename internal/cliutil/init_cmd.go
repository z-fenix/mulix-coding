package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/z-fenix/mulix-coding/internal/scaffold"
)

func newInitCmd() *cobra.Command {
	var force bool
	var dir string
	var hosts []string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install mulix into a project: .mulix/, the mulix and superpowers skills for the selected agent hosts, and the PreToolUse hook (Claude Code only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedHosts, err := scaffold.ParseHosts(hosts)
			if err != nil {
				return err
			}
			res, err := scaffold.Init(scaffold.InitOptions{Root: dir, Force: force, Hosts: parsedHosts})
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
			for _, h := range parsedHosts {
				if h == scaffold.HostClaude {
					fmt.Println("note (Claude Code): the skills are installed as project skills in .claude/skills/. If the superpowers plugin is also enabled, disable it for this project so its SessionStart bootstrap and namespaced skills don't compete with mulix's phase gates.")
				}
				if h == scaffold.HostDSH {
					fmt.Println("note (DeepSeek Harness): the skills are installed as project skills in .dsh/skills/ and a mulix section was merged into AGENTS.md. This host has no PreToolUse hook — the guard checks inside `mulix state transition` are the enforcement; treat a failed guard as blocking.")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files mulix already installed")
	cmd.Flags().StringVar(&dir, "dir", ".", "target project directory")
	cmd.Flags().StringSliceVar(&hosts, "host", []string{"claude"}, "agent host(s) to install skills for: claude, dsh (repeat the flag or separate with commas)")
	return cmd
}
