package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var (
	newBase       string
	newBranchFlag string
	newPRFlag     string
)

func init() {
	newCmd.Flags().StringVar(&newBase, "base", "main", "Base branch to create from")
	newCmd.Flags().StringVarP(&newBranchFlag, "branch", "b", "", "Fetch and track an existing remote branch")
	newCmd.Flags().StringVarP(&newPRFlag, "pr", "P", "", "Checkout a pull request (number, branch, or title)")
	newCmd.MarkFlagsMutuallyExclusive("branch", "pr")

	_ = newCmd.RegisterFlagCompletionFunc("branch", completeRemoteBranchValues)
	_ = newCmd.RegisterFlagCompletionFunc("pr", completePRValues)

	rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new [branch]",
	Short: "Create a new worktree for a branch",
	Long: `Create a new worktree from a branch name, remote branch, or pull request.

Modes (mutually exclusive):
  wtf new <branch>           Create a new branch from --base
  wtf new --branch <name>    Fetch and track an existing remote branch
  wtf new --pr <id>          Checkout a pull request by number, branch, or title`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRemoteBranches,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchNew(cmd, args, newBase, newBranchFlag, newPRFlag, false)
	},
}

// dispatchNew validates the mode and dispatches to the appropriate handler.
// switchMode controls output: when true, path goes to stdout (for shell cd),
// messages to stderr. When false, everything goes to stdout.
func dispatchNew(cmd *cobra.Command, args []string, base, branchFlag, prFlag string, switchMode bool) error {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)
	runner := setup.NewRunner()

	modes := 0
	if len(args) > 0 {
		modes++
	}
	if branchFlag != "" {
		modes++
	}
	if prFlag != "" {
		modes++
	}

	switch {
	case modes == 0:
		return fmt.Errorf("requires a branch name, --branch, or --pr flag")
	case modes > 1:
		return fmt.Errorf("positional branch argument, --branch, and --pr are mutually exclusive")
	case branchFlag != "":
		if cmd.Flags().Changed("base") {
			return fmt.Errorf("--base cannot be used with --branch")
		}
		return runNewBranch(cmd, branchFlag, wm, exec, runner, switchMode)
	case prFlag != "":
		if cmd.Flags().Changed("base") {
			return fmt.Errorf("--base cannot be used with --pr")
		}
		return runNewPR(cmd, prFlag, wm, exec, runner, nil, switchMode)
	default:
		return runNew(cmd, args[0], base, wm, runner, switchMode)
	}
}

func runNew(cmd *cobra.Command, branch, base string, wm *git.WorktreeManager, runner *setup.Runner, switchMode bool) error {
	bm := git.NewBranchManager(&git.RealExecutor{})
	if err := bm.ValidateBranchName(branch); err != nil {
		return err
	}

	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wtPath, err := wm.Add(dir, branch, base)
	if err != nil {
		return err
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]string{
			"path":   wtPath,
			"branch": branch,
		})
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Created worktree at %s\n", greenBold("✔"), cyan(wtPath))

	runPostCreateSetup(cmd, wm, runner, dir, wtPath, branch)

	return nil
}

func runNewBranch(cmd *cobra.Command, branch string, wm *git.WorktreeManager, exec git.Executor, runner *setup.Runner, switchMode bool) error {
	bm := git.NewBranchManager(&git.RealExecutor{})
	if err := bm.ValidateBranchName(branch); err != nil {
		return err
	}

	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	// Fetch the remote branch, creating a local tracking branch
	fetchRef := fmt.Sprintf("%s:%s", branch, branch)
	if _, err := exec.Run(dir, "fetch", "origin", fetchRef); err != nil {
		return fmt.Errorf("fetching remote branch %q: %w", branch, err)
	}

	wtPath, err := wm.Add(dir, branch, branch)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]string{
			"path":   wtPath,
			"branch": branch,
		})
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Created worktree at %s\n", greenBold("✔"), cyan(wtPath))

	runPostCreateSetup(cmd, wm, runner, dir, wtPath, branch)

	return nil
}

// forgeFactory creates a Forge from a remote URL. Abstracted for testability.
type forgeFactory func(remoteURL string) (forge.Forge, error)

func runNewPR(cmd *cobra.Command, arg string, wm *git.WorktreeManager, exec git.Executor, runner *setup.Runner, ff forgeFactory, switchMode bool) error {
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

	fetchRef := f.FetchRef(pr.Number)
	if _, err := exec.Run(dir, "fetch", "origin", fetchRef); err != nil {
		return fmt.Errorf("fetching PR ref: %w", err)
	}

	localBranch := prBranchName(f.Name(), pr.Number)

	wtPath, err := wm.Add(dir, localBranch, localBranch)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"path":   wtPath,
			"branch": localBranch,
			"pr": map[string]any{
				"number": pr.Number,
				"title":  pr.Title,
				"author": pr.Author,
				"url":    prURL,
				"draft":  pr.IsDraft,
			},
		})
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Checked out %s → %s\n",
		greenBold("✔"), prLink, cyan(wtPath))

	// Background validation: verify PR is still open so we can warn about stale data.
	go func() {
		vCtx, vCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer vCancel()
		freshPR, gErr := f.GetPR(vCtx, pr.Number)
		if gErr != nil {
			return
		}
		if freshPR.IsDraft && !pr.IsDraft {
			_, _ = fmt.Fprintf(stderr, "%s PR #%d is now a draft\n", yellow("⚠"), pr.Number)
		}
	}()

	cfg := runPostCreateSetup(cmd, wm, runner, dir, wtPath, localBranch)
	if cfg != nil && len(cfg.Hooks.OnPRCreate) > 0 {
		if runner != nil {
			if hookErr := runner.RunHooks(cfg.Hooks.OnPRCreate, wtPath); hookErr != nil {
				_, _ = fmt.Fprintf(stderr, "%s on_pr_create hook failed: %v\n", yellow("⚠"), hookErr)
			}
		}
	}

	return nil
}

// newOutputWriters returns writers for messages and path output.
// In switch mode: messages go to stderr, path goes to stdout.
// In normal mode: messages go to stdout, path is nil (not printed).
func newOutputWriters(cmd *cobra.Command, switchMode bool) (msgW io.Writer, pathW io.Writer) {
	if switchMode {
		return cmd.ErrOrStderr(), cmd.OutOrStdout()
	}
	return cmd.OutOrStdout(), nil
}

// runPostCreateSetup runs project setup after worktree creation.
// Returns the loaded config (or nil) so callers can run additional hooks.
func runPostCreateSetup(cmd *cobra.Command, wm *git.WorktreeManager, runner *setup.Runner, dir, wtPath, branch string) *config.ProjectConfig {
	if runner == nil {
		return nil
	}

	mainWt, mainErr := wm.MainWorktree(dir)
	if mainErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), mainErr)
		return nil
	}

	cfg, cfgErr := config.LoadProjectConfig(mainWt.Path)
	if cfgErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), cfgErr)
		return nil
	}

	if cfg != nil {
		if valErr := config.ValidateProjectConfig(cfg); valErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), valErr)
			return nil
		}
	}

	if setupErr := runner.RunSetup(cfg, mainWt.Path, wtPath, branch); setupErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup failed: %v\n", yellow("⚠"), setupErr)
	}

	return cfg
}

// resolvePR finds a PR by number, branch name, or title.
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

	// Fall through to title matching if no branch matches
	if len(matches) == 0 {
		argLower := strings.ToLower(arg)
		// First try substring match on title
		for i := range prs {
			if strings.Contains(strings.ToLower(prs[i].Title), argLower) {
				matches = append(matches, &prs[i])
			}
		}
		// If still no matches, try fuzzy match on title
		if len(matches) == 0 {
			for i := range prs {
				if fuzzyScore(strings.ToLower(prs[i].Title), argLower) > 0 {
					matches = append(matches, &prs[i])
				}
			}
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
