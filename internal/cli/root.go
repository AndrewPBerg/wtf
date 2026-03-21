package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "wtf",
	Short:   "WorkTreeForge (WTF) — a fast git worktree workflow tool",
	Long:    "WorkTreeForge (WTF) streamlines git worktree operations, project setup, and forge integrations.",
	Version: Version,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
