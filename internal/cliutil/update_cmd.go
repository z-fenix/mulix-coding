package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/z-fenix/mulix-coding/internal/scaffold"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func newUpdateCmd() *cobra.Command {
	var force bool
	var accept []string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update bundled skills and templates in this project, three-way merging local edits",
		Long: "Refresh skills for the installed agent hosts and templates to the " +
			"versions bundled with this mulix binary (not a CLI self-update). " +
			"Trusted vendor baselines support three-way merging; unknown content " +
			"is retained and reported as blocked. Conflicts retain the active local " +
			"file and vendor baseline, and require manual action. Resolve the sidecar " +
			"candidate into the reported target, then use repeatable --accept <file>; " +
			"accept only advances an existing pending update, not unknown legacy content. " +
			"Force overwrites managed content only after protective backups succeed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}

			res, updateErr := scaffold.Update(scaffold.UpdateOptions{Root: root, Force: force, Accept: accept})
			printUpdateResult(res)
			if updateErr != nil {
				return updateErr
			}
			if res.NeedsAttention() {
				return fmt.Errorf("update requires manual action; resolve the reported conflicts or blocked files")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "back up managed content, then replace it with the bundled version, discarding local edits")
	cmd.Flags().StringArrayVar(&accept, "accept", nil, "accept one manually resolved pending update (project-relative file path; repeatable; not compatible with --force)")
	return cmd
}

func printUpdateResult(res scaffold.UpdateResult) {
	fmt.Printf("update: %d written, %d updated, %d merged, %d conflicted, %d removed, %d skipped, %d accepted, %d migrated, %d blocked, %d partial\n",
		len(res.Written), len(res.Updated), len(res.Merged), len(res.Conflicted), len(res.Removed), len(res.Skipped), len(res.Accepted), len(res.Migrated), len(res.Blocked), len(res.Partial))
	for _, category := range []struct {
		label string
		files []string
	}{
		{"written", res.Written}, {"updated", res.Updated}, {"merged", res.Merged},
		{"conflicted", res.Conflicted}, {"accepted", res.Accepted}, {"migrated", res.Migrated},
		{"removed", res.Removed}, {"blocked", res.Blocked}, {"partial", res.Partial},
	} {
		for _, file := range category.files {
			fmt.Printf("  - %s: %s\n", category.label, file)
		}
	}
	for _, backup := range res.Backups {
		fmt.Printf("  - backup: %s\n", backup)
	}
	printCategory("skipped", res.Skipped)
}

// printCategory caps informational already-current listings only.
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
