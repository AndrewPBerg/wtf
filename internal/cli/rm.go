package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/port"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/AndrewPBerg/wtf/internal/vcs"
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
	Use:               "rmg [branch...]",
	Short:             "Remove worktrees globally (shortcut for rm -g)",
	ValidArgsFunction: completeWorktrees,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runRmInteractiveGlobal(cmd)
		}
		return runRmGlobal(cmd, args)
	},
}

var rmCmd = &cobra.Command{
	Use:               "rm [branch...]",
	Short:             "Remove worktrees without deleting branches",
	ValidArgsFunction: completeWorktrees,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rmGlobal {
			if len(args) == 0 {
				return runRmInteractiveGlobal(cmd)
			}
			return runRmGlobal(cmd, args)
		}
		wm, err := resolveManager(cmd)
		if err != nil {
			return err
		}
		if len(args) == 0 {
			return runRmInteractive(cmd, wm)
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

func runRm(cmd *cobra.Command, branch string, wm vcs.Manager) error {
	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	runOnRemoveHooks(cmd, wm, dir, branch)

	if err := wm.Remove(dir, branch, cwd, rmForce); err != nil {
		return err
	}

	if !jsonOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s for %s\n",
			greenBold("✔"), wm.Kind().Noun(), cyan(branch))
	}
	return nil
}

// runOnRemoveHooks stops the dev server and releases port for the worktree.
func runOnRemoveHooks(cmd *cobra.Command, wm vcs.Manager, repoDir, branch string) {
	// Stop dev server if running in this checkout.
	if wt, err := wm.Find(repoDir, branch); err == nil && wt.Path != "" {
		_ = port.StopDevServer(wt.Path)
	}

	alloc, err := portAllocator(wm, repoDir)
	if err != nil {
		return
	}
	if err := alloc.Release(branch); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s port release failed: %v\n", yellow("⚠"), err)
	}
}

// friendlyError returns a short, user-facing message for known error types,
// stripping noisy git internals.
func friendlyError(err error) string {
	switch {
	case errors.Is(err, vcs.ErrWorktreeHasChanges):
		return "has uncommitted changes — use --force to remove anyway"
	case errors.Is(err, vcs.ErrMainWorktree):
		return "cannot remove main worktree"
	case errors.Is(err, vcs.ErrWorktreeIsCurrentDir):
		return "cannot remove worktree you are currently inside"
	default:
		return err.Error()
	}
}

func runRmGlobal(cmd *cobra.Command, branches []string) error {
	repos, err := loadGlobalRepos()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	type rmGlobalResult struct {
		Branch string `json:"branch"`
		Repo   string `json:"repo,omitempty"`
		VCS    string `json:"vcs,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	stderr := cmd.ErrOrStderr()
	var errs []error
	var jsonResults []rmGlobalResult

	// remove runs a single resolved match and records the outcome. Each match
	// carries its own backend, so nothing has to be re-detected here.
	remove := func(m globalMatch, query string) {
		if rmErr := m.mgr.Remove(m.repo, query, cwd, rmForce); rmErr != nil {
			_, _ = fmt.Fprintf(stderr, "%s failed to remove %s %s: %s\n",
				redBold("✗"), cyan(query), dim("("+m.label()+")"), friendlyError(rmErr))
			errs = append(errs, fmt.Errorf("removing %q from %s: %w", query, m.label(), rmErr))
			if jsonOutput {
				jsonResults = append(jsonResults, rmGlobalResult{
					Branch: query, Repo: m.repo, VCS: m.mgr.Kind().Label(), Error: rmErr.Error(),
				})
			}
			return
		}
		if jsonOutput {
			jsonResults = append(jsonResults, rmGlobalResult{
				Branch: m.wt.Branch, Repo: m.repo, VCS: m.mgr.Kind().Label(),
			})
			return
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s for %s %s\n",
			greenBold("✔"), m.mgr.Kind().Noun(),
			cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
	}

	for _, branch := range branches {
		matches := findGlobal(cmd, repos, branch)

		switch {
		case len(matches) == 1:
			remove(matches[0], branch)

		case len(matches) > 1:
			if jsonOutput {
				// Non-interactive: remove all matches.
				for _, m := range matches {
					remove(m, branch)
				}
				continue
			}
			selected, promptErr := promptMultiRemove(cmd, branch, matches)
			if promptErr != nil {
				errs = append(errs, promptErr)
				continue
			}
			for _, m := range selected {
				remove(m, branch)
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

// promptMultiRemove displays numbered matches and asks the user which to remove.
// Falls back to an error if stdin is not a TTY.
func promptMultiRemove(cmd *cobra.Command, branch string, matches []globalMatch) ([]globalMatch, error) {
	stderr := cmd.ErrOrStderr()

	// Non-interactive: fall back to error
	if !stdinIsTTY() {
		_, _ = fmt.Fprintf(stderr, "%s multiple worktrees match %s across repos:\n", redBold("error:"), cyan(branch))
		for _, m := range matches {
			_, _ = fmt.Fprintf(stderr, "  %s %s %s\n", yellow("→"), cyan(m.wt.Branch), dim("("+m.label()+")"))
		}
		return nil, fmt.Errorf("multiple global matches for %q — use the full branch name to disambiguate", branch)
	}

	_, _ = fmt.Fprintf(stderr, "\n%s multiple worktrees match %s:\n", yellow("?"), cyan(branch))
	for i, m := range matches {
		_, _ = fmt.Fprintf(stderr, "  %s %s %s\n",
			cyanBold(fmt.Sprintf("[%d]", i+1)),
			cyan(m.wt.Branch),
			dim("("+m.label()+")"))
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
	var selected []globalMatch
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

// runRmInteractive launches an interactive multi-select picker for removing worktrees.
func runRmInteractive(cmd *cobra.Command, wm vcs.Manager) error {
	if !stdinIsTTY() {
		return fmt.Errorf("please specify at least one branch name to remove\n\nUsage: wtf rm <branch> [branch...]")
	}

	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()

	wts, err := wm.List(dir)
	if err != nil {
		return err
	}

	// Filter out main worktree and the worktree the user is currently inside.
	items := removablePickerItems(wts, cwd, "", pickerKindLabel(wm, dir))
	if len(items) == 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), dim("No removable worktrees."))
		return nil
	}

	result, err := runPickerFunc(items, true)
	if err != nil {
		return err
	}
	if result.Quit || len(result.Items) == 0 {
		return nil
	}

	var errs []error
	for _, item := range result.Items {
		runOnRemoveHooks(cmd, wm, dir, item.Branch)
		if rmErr := wm.Remove(dir, item.Branch, cwd, rmForce); rmErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s failed to remove %s: %s\n", redBold("✗"), cyan(item.Branch), friendlyError(rmErr))
			errs = append(errs, rmErr)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed worktree for %s\n", greenBold("✔"), cyan(item.Branch))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove %d of %d worktrees", len(errs), len(result.Items))
	}
	return nil
}

// runRmInteractiveGlobal launches an interactive multi-select picker across all repos.
func runRmInteractiveGlobal(cmd *cobra.Command) error {
	if !stdinIsTTY() {
		return fmt.Errorf("please specify at least one branch name to remove\n\nUsage: wtf rmg <branch> [branch...]")
	}

	repos, err := loadGlobalRepos()
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()

	groups := collectGlobal(cmd, repos)

	// Each row is tagged with its backend, so choosing a row chooses the backend
	// too — no prompting needed even for a colocated repo.
	allItems, origin := globalPickerItems(groups, func(_ globalGroup, wt vcs.Worktree) bool {
		if wt.IsMain || wt.IsBare || wt.Branch == "" {
			return false
		}
		return cwd == "" || wt.Path == "" || !isCurrentWorktree(cwd, wt.Path)
	})

	if len(allItems) == 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), dim("No removable worktrees across registered repos."))
		return nil
	}

	result, err := runPickerFunc(allItems, true)
	if err != nil {
		return err
	}
	if result.Quit || len(result.Items) == 0 {
		return nil
	}

	var errs []error
	for _, item := range result.Items {
		m, ok := origin[item.Path]
		if !ok {
			continue
		}
		runOnRemoveHooks(cmd, m.mgr, m.repo, item.Branch)
		if rmErr := m.mgr.Remove(m.repo, item.Branch, cwd, rmForce); rmErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s failed to remove %s %s: %s\n",
				redBold("✗"), cyan(item.Branch), dim("("+m.label()+")"), friendlyError(rmErr))
			errs = append(errs, rmErr)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s for %s %s\n",
				greenBold("✔"), m.mgr.Kind().Noun(),
				cyan(item.Branch), dim("("+filepath.Base(m.repo)+")"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove %d of %d worktrees", len(errs), len(result.Items))
	}
	return nil
}

// removablePickerItems filters worktrees into picker items, excluding main and current directory.
func removablePickerItems(wts []vcs.Worktree, cwd, repo string, kind vcs.Kind) []ui.PickerItem {
	var items []ui.PickerItem
	for _, wt := range wts {
		if wt.IsMain || wt.IsBare || wt.Branch == "" {
			continue
		}
		if cwd != "" && isCurrentWorktree(cwd, wt.Path) {
			continue
		}
		items = append(items, ui.PickerItem{
			Branch: wt.Branch,
			Path:   wt.Path,
			Head:   wt.Head,
			IsMain: wt.IsMain,
			Repo:   repo,
			VCS:    kind.Label(),
		})
	}
	return items
}
