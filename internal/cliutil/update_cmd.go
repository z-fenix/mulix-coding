package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/scaffold"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

func newUpdateCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update bundled skills and templates in this project, three-way merging local edits",
		Long: "Refresh the mulix skills (.claude/skills/*/SKILL.md) and templates " +
			"(.mulix/templates/*) installed in this project to the versions bundled " +
			"with this mulix binary. Files untouched since install are updated in " +
			"place; locally modified files are three-way merged against the baseline " +
			"copy recorded at install time, and real conflicts are left in the file " +
			"as Git-style markers and reported. Files installed without a baseline " +
			"(by an older mulix) are skipped and reported.",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}

			res, err := scaffold.Update(scaffold.UpdateOptions{Root: root, Force: force})
			if err != nil {
				return err
			}

			fmt.Printf("update: %d written, %d updated, %d merged, %d conflicted, %d skipped\n",
				len(res.Written), len(res.Updated), len(res.Merged), len(res.Conflicted), len(res.Skipped))
			printCategory("merged", res.Merged)
			printCategory("conflicted", res.Conflicted)
			printCategory("skipped", res.Skipped)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite every managed file with the bundled version, discarding local edits")
	return cmd
}

// printCategory lists a result category's files, capped so a first-run
// update (everything "written") doesn't flood the terminal.
func printCategory(label string, files []string) {
	const maxListed = 5
	if len(files) == 0 {
		return
	}
	for i, f := range files {
		if i == maxListed {
			fmt.Printf("  - %s: ... and %d more\n", label, len(files)-maxListed)
			return
		}
		fmt.Printf("  - %s: %s\n", label, f)
	}
}
