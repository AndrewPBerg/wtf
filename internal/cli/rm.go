package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var (
	rmForce  bool
	rmGlobal bool
)

func init() {
	rmCmd.Flags().BoolVar(&rmForce, "force", false, "Force remove even with uncommitted changes")
	rmCmd.Flags().BoolVarP(&rmGlobal, "global", "g", false, "Remove worktree across all registered repos")
	rootCmd.AddCommand(rmCmd)
	rmgCmd.Flags().BoolVar(&rmForce, "force", false, "Force remove even with uncommitted changes")
	rootCmd.AddCommand(rmgCmd)
}

var rmgCmd = &cobra.Command{
	Use:   "rmg <branch>",
	Short: "Remove a worktree globally (shortcut for rm -g)",
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify a branch name to remove\n\nUsage: wtf rmg <branch>")
		}
		if len(args) > 1 {
			return fmt.Errorf("expected 1 branch name, got %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRmGlobal(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}))
	},
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
		wm := git.NewWorktreeManager(&git.RealExecutor{})
		if rmGlobal {
			return runRmGlobal(cmd, args[0], wm)
		}
		return runRm(cmd, args[0], wm)
	},
}

func runRm(cmd *cobra.Command, branch string, wm *git.WorktreeManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if err := wm.Remove(dir, branch, cwd, rmForce); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s\n", greenBold("✔"), cyan(branch))
	return nil
}

func runRmGlobal(cmd *cobra.Command, branch string, wm *git.WorktreeManager) error {
	repos, err := config.LoadValid()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no registered repos — run a wtf command inside a repo to auto-register it")
	}

	type match struct {
		wt   git.Worktree
		repo string
	}
	var matches []match

	for _, repo := range repos {
		wt, findErr := wm.Find(repo, branch)
		if findErr == nil {
			matches = append(matches, match{wt: wt, repo: repo})
		}
	}

	if len(matches) == 1 {
		m := matches[0]

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		if err := wm.Remove(m.repo, branch, cwd, rmForce); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s %s\n",
			greenBold("✔"), cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
		return nil
	}

	stderr := cmd.ErrOrStderr()

	if len(matches) > 1 {
		_, _ = fmt.Fprintf(stderr, "%s multiple worktrees match %s across repos:\n", redBold("error:"), cyan(branch))
		for _, m := range matches {
			_, _ = fmt.Fprintf(stderr, "  %s %s %s\n", yellow("→"), cyan(m.wt.Branch), dim("("+m.repo+")"))
		}
		return fmt.Errorf("multiple global matches for %q — use the full branch name to disambiguate", branch)
	}

	_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s across registered repos\n", redBold("error:"), cyan(branch))
	return fmt.Errorf("no global worktree found matching %q", branch)
}
