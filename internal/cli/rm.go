package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var rmForce bool

func init() {
	rmCmd.Flags().BoolVar(&rmForce, "force", false, "Force remove even with uncommitted changes")
	rootCmd.AddCommand(rmCmd)
}

var rmCmd = &cobra.Command{
	Use:   "rm <branch>",
	Short: "Remove a worktree and its branch",
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify a branch name to remove\n\nUsage: wtf rm <branch>")
		}
		if len(args) > 1 {
			return fmt.Errorf("expected 1 branch name, got %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRm(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

func runRm(cmd *cobra.Command, branch string, wm *git.WorktreeManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	if err := wm.Remove(dir, branch, rmForce); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s\n", greenBold("✔"), cyan(branch))
	return nil
}
