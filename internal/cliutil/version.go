package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mulix version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("mulix " + Version)
			return nil
		},
	}
}
