package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var (
	rmForce  bool
	rmGlobal bool
)

func init() {
	rmCmd.Flags().BoolVarP(&rmForce, "force", "F", false, "Force remove even with uncommitted changes")
	rmCmd.Flags().BoolVarP(&rmGlobal, "global", "g", false, "Remove worktree across all registered repos")
	rootCmd.AddCommand(rmCmd)
	rmgCmd.Flags().BoolVarP(&rmForce, "force", "F", false, "Force remove even with uncommitted changes")
	rootCmd.AddCommand(rmgCmd)
}

var rmgCmd = &cobra.Command{
	Use:   "rmg <branch> [branch...]",
	Short: "Remove worktrees globally (shortcut for rm -g)",
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify at least one branch name to remove\n\nUsage: wtf rmg <branch> [branch...]")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRmGlobal(cmd, args, git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

var rmCmd = &cobra.Command{
	Use:   "rm <branch> [branch...]",
	Short: "Remove worktrees and their branches",
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify at least one branch name to remove\n\nUsage: wtf rm <branch> [branch...]")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		wm := git.NewWorktreeManager(&git.RealExecutor{})
		if rmGlobal {
			return runRmGlobal(cmd, args, wm)
		}
		var errs []error
		for _, branch := range args {
			if err := runRm(cmd, branch, wm); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) == 1 {
			return errs[0]
		}
		if len(errs) > 1 {
			return fmt.Errorf("failed to remove %d of %d worktrees", len(errs), len(args))
		}
		return nil
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

	runOnRemoveHooks(cmd, dir)

	if err := wm.Remove(dir, branch, cwd, rmForce); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s\n", greenBold("✔"), cyan(branch))
	return nil
}

// runOnRemoveHooks loads config and runs on_remove hooks if present.
// Failures are logged as warnings, never fatal.
func runOnRemoveHooks(cmd *cobra.Command, repoDir string) {
	cfg, err := config.LoadProjectConfig(repoDir)
	if err != nil || cfg == nil || len(cfg.Hooks.OnRemove) == 0 {
		return
	}

	runner := setup.NewRunner()
	if err := runner.RunHooks(cfg.Hooks.OnRemove, repoDir); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s on_remove hook failed: %v\n", yellow("⚠"), err)
	}
}

// friendlyError returns a short, user-facing message for known error types,
// stripping noisy git internals.
func friendlyError(err error) string {
	switch {
	case errors.Is(err, git.ErrWorktreeHasChanges):
		return "has uncommitted changes — use --force to remove anyway"
	case errors.Is(err, git.ErrMainWorktree):
		return "cannot remove main worktree"
	case errors.Is(err, git.ErrWorktreeIsCurrentDir):
		return "cannot remove worktree you are currently inside"
	default:
		return err.Error()
	}
}

func runRmGlobal(cmd *cobra.Command, branches []string, wm *git.WorktreeManager) error {
	repos, err := config.LoadValid()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no registered repos — run a wtf command inside a repo to auto-register it")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	type match struct {
		wt   git.Worktree
		repo string
	}

	stderr := cmd.ErrOrStderr()
	var errs []error

	for _, branch := range branches {
		var matches []match
		for _, repo := range repos {
			wt, findErr := wm.Find(repo, branch)
			if findErr == nil {
				matches = append(matches, match{wt: wt, repo: repo})
			}
		}

		switch {
		case len(matches) == 1:
			m := matches[0]
			if rmErr := wm.Remove(m.repo, branch, cwd, rmForce); rmErr != nil {
				_, _ = fmt.Fprintf(stderr, "%s failed to remove %s: %s\n", redBold("✗"), cyan(branch), friendlyError(rmErr))
				errs = append(errs, fmt.Errorf("removing %q: %w", branch, rmErr))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s %s\n",
					greenBold("✔"), cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
			}

		case len(matches) > 1:
			_, _ = fmt.Fprintf(stderr, "%s multiple worktrees match %s across repos:\n", redBold("error:"), cyan(branch))
			for _, m := range matches {
				_, _ = fmt.Fprintf(stderr, "  %s %s %s\n", yellow("→"), cyan(m.wt.Branch), dim("("+m.repo+")"))
			}
			errs = append(errs, fmt.Errorf("multiple global matches for %q — use the full branch name to disambiguate", branch))

		default:
			_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s across registered repos\n", redBold("error:"), cyan(branch))
			errs = append(errs, fmt.Errorf("no global worktree found matching %q", branch))
		}
	}

	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return fmt.Errorf("failed to remove %d of %d worktrees", len(errs), len(branches))
	}
	return nil
}
