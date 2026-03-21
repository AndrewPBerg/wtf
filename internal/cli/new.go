package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var newBase string

func init() {
	newCmd.Flags().StringVar(&newBase, "base", "main", "Base branch to create from")
	rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new <branch>",
	Short: "Create a new worktree for a branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNew(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

func runNew(cmd *cobra.Command, branch string, wm *git.WorktreeManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wtPath, err := wm.Add(dir, branch, newBase)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Created worktree at %s\n", greenBold("✔"), cyan(wtPath))
	return nil
}
