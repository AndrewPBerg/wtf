package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(prCmd)
}

var prCmd = &cobra.Command{
	Use:   "pr <number|branch>",
	Short: "Checkout a pull request as a worktree",
	Long: `Checkout a pull request (or merge request) as a new worktree.

Accepts a PR number or branch name. Detects GitHub/GitLab automatically
from the origin remote and reuses your gh/glab authentication.

Examples:
  wtf pr 42          # checkout PR #42
  wtf pr feature-x   # checkout PR by branch name`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completePR,
	RunE: func(cmd *cobra.Command, args []string) error {
		exec := &git.RealExecutor{}
		wm := git.NewWorktreeManager(exec)
		runner := setup.NewRunner()
		return runPR(cmd, args[0], wm, exec, runner, nil)
	},
}

// completePR provides tab-completion for PR numbers/branches.
func completePR(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	dbg, _ := os.OpenFile("/tmp/wtf-comp.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	defer func() {
		if dbg != nil {
			_ = dbg.Close()
		}
	}()
	logf := func(format string, a ...any) {
		if dbg != nil {
			_, _ = fmt.Fprintf(dbg, format+"\n", a...)
		}
	}

	dir, err := getRepoDir()
	if err != nil {
		logf("getRepoDir: %v", err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	logf("dir: %s", dir)

	remoteURL, err := wm.RemoteURL(dir)
	if err != nil {
		logf("RemoteURL: %v", err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	logf("remoteURL: %s", remoteURL)

	f, err := forge.Detect(remoteURL)
	if err != nil {
		logf("Detect: %v", err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	logf("forge: %s", f.Name())

	// Try to use cache for fast completions
	gitCommonDir, gcErr := exec.Run(dir, "rev-parse", "--git-common-dir")
	if gcErr == nil {
		f = forge.NewCachedForge(f, gitCommonDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prs, err := f.ListPRs(ctx)
	if err != nil {
		logf("ListPRs: %v", err)
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	logf("prs: %d", len(prs))

	var completions []string
	for _, pr := range prs {
		desc := fmt.Sprintf("%d\t#%d %s (%s)", pr.Number, pr.Number, pr.Branch, pr.Author)
		completions = append(completions, desc)
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// forgeFactory creates a Forge from a remote URL. Abstracted for testability.
type forgeFactory func(remoteURL string) (forge.Forge, error)

func runPR(cmd *cobra.Command, arg string, wm *git.WorktreeManager, exec git.Executor, runner *setup.Runner, ff forgeFactory) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	remoteURL, err := wm.RemoteURL(dir)
	if err != nil {
		return fmt.Errorf("getting remote URL: %w", err)
	}

	if ff == nil {
		ff = func(remote string) (forge.Forge, error) {
			f, fErr := forge.Detect(remote)
			if fErr != nil {
				return nil, fErr
			}
			gitCommonDir, gcErr := exec.Run(dir, "rev-parse", "--git-common-dir")
			if gcErr != nil {
				return f, nil
			}
			return forge.NewCachedForge(f, gitCommonDir), nil
		}
	}

	f, err := ff(remoteURL)
	if err != nil {
		return fmt.Errorf("detecting forge: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pr, err := resolvePR(ctx, f, arg)
	if err != nil {
		return err
	}

	stderr := cmd.ErrOrStderr()

	prLabel := fmt.Sprintf("#%d", pr.Number)
	prURL := f.PRURL(pr.Number)
	prLink := hyperlink(prURL, cyan(prLabel))

	_, _ = fmt.Fprintf(stderr, "Fetching %s %s…\n", prLink, dim(pr.Title))

	// Fetch the PR ref
	fetchRef := f.FetchRef(pr.Number)
	if _, err := exec.Run(dir, "fetch", "origin", fetchRef); err != nil {
		return fmt.Errorf("fetching PR ref: %w", err)
	}

	// Use the local ref created by the fetch (pr-N or mr-N)
	localBranch := prBranchName(f.Name(), pr.Number)

	// Create worktree from the fetched ref
	wtPath, err := wm.Add(dir, localBranch, localBranch)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Checked out %s → %s\n",
		greenBold("✔"), prLink, cyan(wtPath))

	// Run setup — failures are warnings, not errors
	if runner != nil {
		mainWt, mainErr := wm.MainWorktree(dir)
		if mainErr == nil {
			cfg, cfgErr := config.LoadProjectConfig(mainWt.Path)
			if cfgErr == nil && cfg != nil {
				if valErr := config.ValidateProjectConfig(cfg); valErr == nil {
					if setupErr := runner.RunSetup(cfg, mainWt.Path, wtPath, localBranch); setupErr != nil {
						_, _ = fmt.Fprintf(stderr, "%s setup failed: %v\n", yellow("⚠"), setupErr)
					}
					if len(cfg.Hooks.OnPRCreate) > 0 {
						if hookErr := runner.RunHooks(cfg.Hooks.OnPRCreate, wtPath); hookErr != nil {
							_, _ = fmt.Fprintf(stderr, "%s on_pr_create hook failed: %v\n", yellow("⚠"), hookErr)
						}
					}
				}
			}
		}
	}

	return nil
}

// resolvePR finds a PR by number or by branch name.
func resolvePR(ctx context.Context, f forge.Forge, arg string) (*forge.PR, error) {
	if num, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && num > 0 {
		pr, err := f.GetPR(ctx, num)
		if err != nil {
			return nil, fmt.Errorf("PR #%d not found: %w", num, err)
		}
		return pr, nil
	}

	prs, err := f.ListPRs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing PRs: %w", err)
	}

	for i := range prs {
		if prs[i].Branch == arg {
			return &prs[i], nil
		}
	}

	var matches []*forge.PR
	for i := range prs {
		if strings.Contains(prs[i].Branch, arg) {
			matches = append(matches, &prs[i])
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no open PR found matching %q", arg)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("#%d (%s)", m.Number, m.Branch))
		}
		return nil, fmt.Errorf("multiple PRs match %q: %s", arg, strings.Join(names, ", "))
	}
}

// prBranchName returns the local branch name for a PR.
func prBranchName(forgeName string, number int) string {
	switch forgeName {
	case "gitlab":
		return fmt.Sprintf("mr-%d", number)
	default:
		return fmt.Sprintf("pr-%d", number)
	}
}
