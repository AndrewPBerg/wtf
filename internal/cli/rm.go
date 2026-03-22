package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/mattn/go-isatty"
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
	Use:               "rmg <branch> [branch...]",
	Short:             "Remove worktrees globally (shortcut for rm -g)",
	ValidArgsFunction: completeWorktrees,
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
	Use:               "rm <branch> [branch...]",
	Short:             "Remove worktrees and their branches",
	ValidArgsFunction: completeWorktrees,
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
		type rmResult struct {
			Branch string `json:"branch"`
			Error  string `json:"error,omitempty"`
		}
		var results []rmResult
		var errs []error
		for _, branch := range args {
			if err := runRm(cmd, branch, wm); err != nil {
				errs = append(errs, err)
				if jsonOutput {
					results = append(results, rmResult{Branch: branch, Error: err.Error()})
				}
			} else if jsonOutput {
				results = append(results, rmResult{Branch: branch})
			}
		}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"removed": results})
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

	runOnRemoveHooks(cmd, dir, branch)

	if err := wm.Remove(dir, branch, cwd, rmForce); err != nil {
		return err
	}

	if !jsonOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s\n", greenBold("✔"), cyan(branch))
	}
	return nil
}

// runOnRemoveHooks loads config and runs on_remove hooks if present.
// If the branch is a PR worktree (pr-N or mr-N), on_pr_delete hooks also run.
// Failures are logged as warnings, never fatal.
func runOnRemoveHooks(cmd *cobra.Command, repoDir string, branch string) {
	cfg, err := config.LoadProjectConfig(repoDir)
	if err != nil || cfg == nil {
		return
	}

	runner := setup.NewRunner()

	if len(cfg.Hooks.OnRemove) > 0 {
		if err := runner.RunHooks(cfg.Hooks.OnRemove, repoDir); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s on_remove hook failed: %v\n", yellow("⚠"), err)
		}
	}

	if isPRBranch(branch) && len(cfg.Hooks.OnPRDelete) > 0 {
		if err := runner.RunHooks(cfg.Hooks.OnPRDelete, repoDir); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s on_pr_delete hook failed: %v\n", yellow("⚠"), err)
		}
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

	type rmGlobalResult struct {
		Branch string `json:"branch"`
		Repo   string `json:"repo,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	stderr := cmd.ErrOrStderr()
	var errs []error
	var jsonResults []rmGlobalResult

	for _, branch := range branches {
		var matches []rmMatch
		for _, repo := range repos {
			wt, findErr := wm.Find(repo, branch)
			if findErr == nil {
				matches = append(matches, rmMatch{wt: wt, repo: repo})
			}
		}

		switch {
		case len(matches) == 1:
			m := matches[0]
			if rmErr := wm.Remove(m.repo, branch, cwd, rmForce); rmErr != nil {
				_, _ = fmt.Fprintf(stderr, "%s failed to remove %s: %s\n", redBold("✗"), cyan(branch), friendlyError(rmErr))
				errs = append(errs, fmt.Errorf("removing %q: %w", branch, rmErr))
				if jsonOutput {
					jsonResults = append(jsonResults, rmGlobalResult{Branch: branch, Repo: m.repo, Error: rmErr.Error()})
				}
			} else {
				if jsonOutput {
					jsonResults = append(jsonResults, rmGlobalResult{Branch: m.wt.Branch, Repo: m.repo})
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s %s\n",
						greenBold("✔"), cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
				}
			}

		case len(matches) > 1:
			if jsonOutput {
				// Non-interactive: remove all matches
				for _, m := range matches {
					if rmErr := wm.Remove(m.repo, branch, cwd, rmForce); rmErr != nil {
						errs = append(errs, fmt.Errorf("removing %q from %s: %w", branch, filepath.Base(m.repo), rmErr))
						jsonResults = append(jsonResults, rmGlobalResult{Branch: branch, Repo: m.repo, Error: rmErr.Error()})
					} else {
						jsonResults = append(jsonResults, rmGlobalResult{Branch: m.wt.Branch, Repo: m.repo})
					}
				}
			} else {
				selected, promptErr := promptMultiRemove(cmd, branch, matches)
				if promptErr != nil {
					errs = append(errs, promptErr)
					continue
				}
				for _, m := range selected {
					if rmErr := wm.Remove(m.repo, branch, cwd, rmForce); rmErr != nil {
						_, _ = fmt.Fprintf(stderr, "%s failed to remove %s: %s\n", redBold("✗"), cyan(branch), friendlyError(rmErr))
						errs = append(errs, fmt.Errorf("removing %q from %s: %w", branch, filepath.Base(m.repo), rmErr))
					} else {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s %s\n",
							greenBold("✔"), cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
					}
				}
			}

		default:
			_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s across registered repos\n", redBold("error:"), cyan(branch))
			errs = append(errs, fmt.Errorf("no global worktree found matching %q", branch))
			if jsonOutput {
				jsonResults = append(jsonResults, rmGlobalResult{Branch: branch, Error: "no matching worktree found"})
			}
		}
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]any{"removed": jsonResults})
	}

	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return fmt.Errorf("failed to remove %d of %d worktrees", len(errs), len(branches))
	}
	return nil
}

type rmMatch struct {
	wt   git.Worktree
	repo string
}

// promptMultiRemove displays numbered matches and asks the user which to remove.
// Falls back to an error if stdin is not a TTY.
func promptMultiRemove(cmd *cobra.Command, branch string, matches []rmMatch) ([]rmMatch, error) {
	stderr := cmd.ErrOrStderr()

	// Non-interactive: fall back to error
	if !stdinIsTTY() {
		_, _ = fmt.Fprintf(stderr, "%s multiple worktrees match %s across repos:\n", redBold("error:"), cyan(branch))
		for _, m := range matches {
			_, _ = fmt.Fprintf(stderr, "  %s %s %s\n", yellow("→"), cyan(m.wt.Branch), dim("("+m.repo+")"))
		}
		return nil, fmt.Errorf("multiple global matches for %q — use the full branch name to disambiguate", branch)
	}

	_, _ = fmt.Fprintf(stderr, "\n%s multiple worktrees match %s:\n", yellow("?"), cyan(branch))
	for i, m := range matches {
		_, _ = fmt.Fprintf(stderr, "  %s %s %s\n",
			cyanBold(fmt.Sprintf("[%d]", i+1)),
			cyan(m.wt.Branch),
			dim("("+filepath.Base(m.repo)+")"))
	}
	_, _ = fmt.Fprintf(stderr, "\nRemove which? [1-%d, all, none] %s ", len(matches), dim("(default: none)"))

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return nil, nil
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "" || strings.EqualFold(input, "none") {
		_, _ = fmt.Fprintf(stderr, "%s skipped %s\n", dim("—"), cyan(branch))
		return nil, nil
	}

	if strings.EqualFold(input, "all") {
		return matches, nil
	}

	// Parse comma-separated indices
	parts := strings.Split(input, ",")
	seen := make(map[int]bool)
	var selected []rmMatch
	for _, p := range parts {
		p = strings.TrimSpace(p)
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(matches) {
			_, _ = fmt.Fprintf(stderr, "%s invalid selection %q — skipping %s\n", redBold("✗"), p, cyan(branch))
			return nil, nil
		}
		if !seen[idx] {
			seen[idx] = true
			selected = append(selected, matches[idx-1])
		}
	}
	return selected, nil
}

// stdinIsTTY reports whether os.Stdin is a terminal.
// Declared as a variable so tests can override it.
var stdinIsTTY = func() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}
