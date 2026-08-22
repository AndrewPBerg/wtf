package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

var swPRs bool

var swGlobal bool

func init() {
	swCmd.Flags().BoolVarP(&swGlobal, "global", "g", false, "Search across all registered repos")
	swCmd.Flags().BoolVarP(&swPRs, "prs", "p", false, "Show PR status for each worktree (list mode)")
	rootCmd.AddCommand(swCmd)
	swgCmd.Flags().BoolVarP(&swPRs, "prs", "p", false, "Show PR status for each worktree (list mode)")
	rootCmd.AddCommand(swgCmd)
}

var swgCmd = &cobra.Command{
	Use:               "swg [branch]",
	Short:             "Switch to a worktree globally (shortcut for sw -g)",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktrees,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			lsGlobal = true
			lsPRs = swPRs
			return runLsGlobal(cmd)
		}
		return runSwGlobal(cmd, args[0])
	},
}

var swCmd = &cobra.Command{
	Use:               "sw [branch]",
	Short:             "Switch to a worktree (prints path for cd)",
	ValidArgsFunction: completeWorktrees,
	Long: `Switch to a worktree by branch name (substring match).
With no arguments, lists all worktrees in an interactive picker.
Prints the worktree path to stdout so you can cd to it.

To enable the 'sw' shell function that cds automatically, run:

  wtf setup shell

Or add this to your shell profile manually:

  eval "$(wtf init)"

See 'wtf init --help' and 'wtf setup --help' for details.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lsPRs = swPRs
		if swGlobal {
			lsGlobal = true
			if len(args) == 0 {
				return runLsGlobal(cmd)
			}
			return runSwGlobal(cmd, args[0])
		}
		lsGlobal = false
		wm, err := resolveManager(cmd)
		if err != nil {
			return err
		}
		if len(args) == 0 {
			return runLs(cmd, wm)
		}
		return runSw(cmd, args[0], wm)
	},
}

func resolveWorktree(dir, query string, wm vcs.Manager) (vcs.Worktree, error) {
	wts, err := wm.List(dir)
	if err != nil {
		return vcs.Worktree{}, err
	}
	wts, err = enrichWorktrees(wm, dir, wts)
	if err != nil {
		return vcs.Worktree{}, err
	}
	if identity.ValidateID(query) == nil {
		return vcs.FindWorkspaceByID(wts, query)
	}
	found, err := wm.Find(dir, query)
	if err != nil {
		return vcs.Worktree{}, err
	}
	for _, wt := range wts {
		if wt.Path == found.Path {
			return wt, nil
		}
	}
	return found, nil
}

func nativeWorktreeRef(wt vcs.Worktree) string {
	if wt.VCS == vcs.KindJJ {
		return wt.NativeName
	}
	return wt.Branch
}

func identityJSON(wt vcs.Worktree) map[string]string {
	result := make(map[string]string, 4)
	if wt.RepositoryID != "" {
		result["repository_id"] = wt.RepositoryID
	}
	if wt.WorkspaceID != "" {
		result["workspace_id"] = wt.WorkspaceID
	}
	if wt.Name != "" {
		result["name"] = wt.Name
	}
	if wt.NativeName != "" {
		result["native_name"] = wt.NativeName
	}
	return result
}

func runSw(cmd *cobra.Command, query string, wm vcs.Manager) error {
	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	wt, err := resolveWorktree(dir, query, wm)
	if err == nil {
		if jsonOutput {
			result := identityJSON(wt)
			result["path"] = wt.Path
			result["branch"] = wt.Branch
			return writeJSON(cmd.OutOrStdout(), result)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), wt.Path)
		if cwd != "" && isCurrentWorktree(cwd, wt.Path) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "wtf? you are already on %s!\n", cyan(wt.Branch))
			return nil
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Switched to %s\n", wt.Path)
		runOnSwitchHooks(cmd, dir, nativeWorktreeRef(wt))
		return nil
	}

	// On error, show colored message with available branches
	stderr := cmd.ErrOrStderr()

	// A colocated repo can hold the target under the other backend; saying so
	// beats a bare "not found".
	hintOtherBackendMatch(cmd, wm, dir, query)

	wts, listErr := wm.List(dir)
	if listErr != nil || len(wts) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s\n", redBold("error:"), cyan(query))
		// Return the original error so the shell function gets a non-zero exit
		return err
	}

	// Collect branch names
	var branches []string
	for _, w := range wts {
		if w.Branch != "" && !w.IsBare {
			branches = append(branches, w.Branch)
		}
	}

	similar := fuzzyFilter(branches, query)

	_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s\n", redBold("error:"), cyan(query))

	if len(similar) > 0 {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", dim("Did you mean?"))
		for _, b := range similar {
			_, _ = fmt.Fprintf(stderr, "  %s %s\n", yellow("→"), cyan(b))
		}
	} else if len(branches) > 0 {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", dim("Available worktrees:"))
		for _, b := range branches {
			_, _ = fmt.Fprintf(stderr, "  %s %s\n", dim("•"), cyan(b))
		}
	}

	// Return the original error for non-zero exit code
	return err
}

func runSwGlobal(cmd *cobra.Command, query string) error {
	repos, err := loadGlobalRepos()
	if err != nil {
		return err
	}

	matches, findErr := findGlobalStrict(cmd, repos, query)
	if findErr != nil {
		return findErr
	}

	if len(matches) == 1 {
		m := matches[0]
		if jsonOutput {
			result := identityJSON(m.wt)
			result["path"] = m.wt.Path
			result["branch"] = m.wt.Branch
			result["repo"] = m.repo
			result["vcs"] = m.mgr.Kind().Label()
			return writeJSON(cmd.OutOrStdout(), result)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), m.wt.Path)
		cwd, _ := os.Getwd()
		if cwd != "" && isCurrentWorktree(cwd, m.wt.Path) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "wtf? you are already on %s!\n", cyan(m.wt.Branch))
			return nil
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Switched to %s\n", m.wt.Path)
		runOnSwitchHooks(cmd, m.repo, nativeWorktreeRef(m.wt))
		return nil
	}

	stderr := cmd.ErrOrStderr()

	if len(matches) > 1 {
		// Matches are labeled with their backend as well as their repo: the same
		// name can exist as a git worktree and a jj workspace in one colocated repo.
		_, _ = fmt.Fprintf(stderr, "%s multiple worktrees match %s:\n", redBold("error:"), cyan(query))
		for _, m := range matches {
			_, _ = fmt.Fprintf(stderr, "  %s %s %s\n", yellow("→"), cyan(m.wt.Branch), dim("("+m.label()+")"))
		}
		return fmt.Errorf("multiple global matches for %q", query)
	}

	// No matches — collect every name across repos for suggestions.
	_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s across registered repos\n", redBold("error:"), cyan(query))

	var allBranches []string
	groups, collectErr := collectGlobalStrict(cmd, repos)
	if collectErr != nil {
		return collectErr
	}
	for _, g := range groups {
		for _, w := range g.wts {
			if w.Branch != "" && !w.IsBare {
				allBranches = append(allBranches, w.Branch)
			}
		}
	}

	similar := fuzzyFilter(allBranches, query)
	if len(similar) > 0 {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", dim("Did you mean?"))
		for _, b := range similar {
			_, _ = fmt.Fprintf(stderr, "  %s %s\n", yellow("→"), cyan(b))
		}
	}

	return fmt.Errorf("no global worktree found matching %q", query)
}

// fuzzyFilter returns branches that are similar to the query.
// Queries shorter than 2 characters are too ambiguous for fuzzy matching.
func fuzzyFilter(branches []string, query string) []string {
	if len(query) < 2 {
		return nil
	}
	query = strings.ToLower(query)
	type scored struct {
		branch string
		score  int
	}
	var results []scored

	for _, b := range branches {
		bl := strings.ToLower(b)

		// Exact substring match — already handled by Find, skip
		if strings.Contains(bl, query) {
			continue
		}

		score := fuzzyScore(bl, query)
		if score > 0 {
			results = append(results, scored{b, score})
		}
	}

	// Sort by score descending
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	// Return top 5
	var out []string
	for i, r := range results {
		if i >= 5 {
			break
		}
		out = append(out, r.branch)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// fuzzyScore scores how well query characters appear in order in target.
func fuzzyScore(target, query string) int {
	ti := 0
	matched := 0
	for qi := 0; qi < len(query) && ti < len(target); qi++ {
		for ti < len(target) {
			if target[ti] == query[qi] {
				matched++
				ti++
				break
			}
			ti++
		}
	}

	if matched == 0 {
		return 0
	}

	// Require at least 40% of query chars to match in sequence
	threshold := len(query) * 2 / 5
	if threshold < 1 {
		threshold = 1
	}
	if matched < threshold {
		return 0
	}

	return matched
}

// isCurrentWorktree returns true if cwd is inside the given worktree path.
func isCurrentWorktree(cwd, wtPath string) bool {
	// Resolve symlinks for reliable comparison
	cwdReal, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		cwdReal = cwd
	}
	wtReal, err := filepath.EvalSymlinks(wtPath)
	if err != nil {
		wtReal = wtPath
	}
	// Check if cwd is the worktree path or a subdirectory of it
	rel, err := filepath.Rel(wtReal, cwdReal)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// runOnSwitchHooks is a no-op placeholder for future CLI-driven hooks.
func runOnSwitchHooks(_ *cobra.Command, _ string, _ string) {}

// isPRBranch returns true if the branch name matches the PR worktree pattern (pr-N or mr-N).
func isPRBranch(branch string) bool {
	if rest, ok := strings.CutPrefix(branch, "pr-"); ok {
		_, err := strconv.Atoi(rest)
		return err == nil
	}
	if rest, ok := strings.CutPrefix(branch, "mr-"); ok {
		_, err := strconv.Atoi(rest)
		return err == nil
	}
	return false
}
