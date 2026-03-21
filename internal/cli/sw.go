package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(swCmd)
}

var swCmd = &cobra.Command{
	Use:   "sw <branch>",
	Short: "Switch to a worktree (prints path for cd)",
	Long: `Switch to a worktree by branch name (substring match).
Prints the worktree path to stdout so you can cd to it.

Shell wrapper for your profile:
  wt() { cd "$(command wtf sw "$@")" }`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSw(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

func runSw(cmd *cobra.Command, query string, wm *git.WorktreeManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wt, err := wm.Find(dir, query)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), wt.Path)
	return nil
}
