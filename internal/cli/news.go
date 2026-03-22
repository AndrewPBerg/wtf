package cli

import (
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var (
	newsBase       string
	newsBranchFlag string
	newsPRFlag     string
)

func init() {
	newsCmd.Flags().StringVar(&newsBase, "base", "main", "Base branch to create from")
	newsCmd.Flags().StringVarP(&newsBranchFlag, "branch", "b", "", "Fetch and track an existing remote branch")
	newsCmd.Flags().StringVarP(&newsPRFlag, "pr", "P", "", "Checkout a pull request (number, branch, or title)")
	newsCmd.MarkFlagsMutuallyExclusive("branch", "pr")

	_ = newsCmd.RegisterFlagCompletionFunc("branch", completeRemoteBranchValues)
	_ = newsCmd.RegisterFlagCompletionFunc("pr", completePRValues)

	rootCmd.AddCommand(newsCmd)
}

var newsCmd = &cobra.Command{
	Use:   "news [branch]",
	Short: "Create a new worktree and switch to it",
	Long: `Create a new worktree and switch to it in one step.

Modes (mutually exclusive):
  wtf news <branch>           Create a new branch from --base and switch
  wtf news --branch <name>    Fetch remote branch and switch
  wtf news --pr <id>          Checkout a pull request and switch`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchNew(cmd, args, newsBase, newsBranchFlag, newsPRFlag, true)
	},
}

// runNews is the thin wrapper for tests that need to call the positional-arg
// path directly, since tests set the base variable rather than using flags.
func runNews(cmd *cobra.Command, branch string, wm *git.WorktreeManager, runner *setup.Runner) error {
	return runNew(cmd, branch, newsBase, wm, runner, true)
}
