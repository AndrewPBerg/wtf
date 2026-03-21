package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "WorkTreeForage (W2F) — a fast git worktree workflow tool",
	Long:  "WorkTreeForage (W2F) streamlines git worktree operations, project setup, and forge integrations.",
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
