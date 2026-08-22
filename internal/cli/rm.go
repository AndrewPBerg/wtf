package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/identity"
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
			Branch       string `json:"branch"`
			RepositoryID string `json:"repository_id,omitempty"`
			WorkspaceID  string `json:"workspace_id,omitempty"`
			Name         string `json:"name,omitempty"`
			NativeName   string `json:"native_name,omitempty"`
			Error        string `json:"error,omitempty"`
			Noop         bool   `json:"noop,omitempty"`
		}
		var results []rmResult
		var errs []error
		for _, branch := range args {
			var wt vcs.Worktree
			if jsonOutput {
				dir, resolveErr := repoDirFor(wm)
				if resolveErr == nil {
					wt, _ = resolveRemovalWorktree(dir, branch, wm)
				}
			}
			if err := runRm(cmd, branch, wm); err != nil {
				errs = append(errs, err)
				if jsonOutput {
					results = append(results, rmResult{Branch: branch, RepositoryID: wt.RepositoryID, WorkspaceID: wt.WorkspaceID, Name: wt.Name, NativeName: wt.NativeName, Error: err.Error()})
				}
			} else if jsonOutput {
				result := rmResult{Branch: branch, RepositoryID: wt.RepositoryID, WorkspaceID: wt.WorkspaceID, Name: wt.Name, NativeName: wt.NativeName}
				if wt.WorkspaceID != "" {
					if store, storeErr := removalIdentityStoreFactory(); storeErr == nil {
						if workspace, lookupErr := store.LookupWorkspace(wt.WorkspaceID); lookupErr == nil {
							result.Noop = workspace.LifecycleState == identity.Removed
						}
					}
				}
				results = append(results, result)
			}
		}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"version": 1, "removed": results})
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

type removalIdentityStore interface {
	LookupWorkspace(string) (identity.Workspace, error)
	LookupRepository(string) (identity.Repository, error)
	RemoveWorkspace(string) (identity.Workspace, error)
	MarkCleanupFailed(string) (identity.Workspace, error)
	FinalizeCleanup(string) (identity.Workspace, error)
}

var removalIdentityStoreFactory = func() (removalIdentityStore, error) {
	return identity.DefaultStore()
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

	wt, err := resolveRemovalWorktree(dir, branch, wm)
	if err != nil {
		return err
	}
	ref := nativeWorktreeRef(wt)

	// UUID retries after successful cleanup are durable, structured no-ops. Do
	// not consult or mutate VCS state for a removed tombstone.
	if jsonOutput && wt.WorkspaceID != "" {
		store, storeErr := removalIdentityStoreFactory()
		if storeErr == nil {
			if workspace, lookupErr := store.LookupWorkspace(wt.WorkspaceID); lookupErr == nil && workspace.LifecycleState == identity.Removed {
				return nil
			}
		}
	}

	if err := removePhysicalAndIdentity(cmd, wm, dir, ref, cwd, wt); err != nil {
		return err
	}

	if !jsonOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s for %s\n",
			greenBold("✔"), wm.Kind().Noun(), cyan(branch))
	}
	return nil
}

// resolveRemovalWorktree permits only UUID-directed repair to consult identity.
// Human selectors remain entirely VCS-based.
func resolveRemovalWorktree(repoDir, query string, wm vcs.Manager) (vcs.Worktree, error) {
	wt, vcsErr := resolveWorktree(repoDir, query, wm)
	if identity.ValidateID(query) != nil {
		return wt, vcsErr
	}
	store, err := removalIdentityStoreFactory()
	if err != nil {
		return vcs.Worktree{}, fmt.Errorf("resolving workspace UUID %s: %w", query, err)
	}
	workspace, lookupErr := store.LookupWorkspace(query)
	if lookupErr != nil {
		return vcs.Worktree{}, vcsErr
	}
	if workspace.Backend != string(wm.Kind()) {
		return vcs.Worktree{}, fmt.Errorf("workspace %s belongs to %s, not %s", query, workspace.Backend, wm.Kind().Label())
	}
	stateDir, stateErr := wm.StateDir(repoDir)
	marker, markerErr := "", stateErr
	if markerErr == nil {
		marker, markerErr = identity.ReadRepositoryID(stateDir)
	}
	if markerErr == nil {
		if workspace.RepositoryID != marker {
			return vcs.Worktree{}, fmt.Errorf("workspace %s does not belong to this repository", query)
		}
	} else {
		repo, repoErr := store.LookupRepository(repoDir)
		if repoErr != nil || repo.ID != workspace.RepositoryID {
			return vcs.Worktree{}, fmt.Errorf("workspace %s cannot be scoped to this repository; use its registered repository", query)
		}
	}
	canonical, err := identity.CanonicalPhysicalPath(workspace.Path)
	if err != nil {
		return vcs.Worktree{}, fmt.Errorf("invalid identity path for workspace %s: %w", query, err)
	}
	if vcsErr == nil {
		physical, pathErr := identity.CanonicalPhysicalPath(wt.Path)
		if pathErr != nil || physical != canonical {
			return vcs.Worktree{}, fmt.Errorf("workspace UUID %s does not match the VCS checkout", query)
		}
		wt.RepositoryID, wt.WorkspaceID = workspace.RepositoryID, workspace.ID
		wt.Name, wt.NativeName = workspace.Name, workspace.NativeName
		return wt, nil
	}
	if _, statErr := os.Lstat(canonical); statErr == nil {
		return vcs.Worktree{}, fmt.Errorf("cannot repair workspace %s: physical path %s still exists but %s cannot resolve it; repair the VCS registration, then retry", query, canonical, wm.Kind().Label())
	} else if !os.IsNotExist(statErr) {
		return vcs.Worktree{}, fmt.Errorf("checking physical path for workspace %s: %w", query, statErr)
	}
	return vcs.Worktree{Path: canonical, RepositoryID: workspace.RepositoryID, WorkspaceID: workspace.ID, Name: workspace.Name, NativeName: workspace.NativeName, Branch: workspace.NativeName, VCS: wm.Kind()}, nil
}

// removePhysicalAndIdentity keeps physical cleanup and the durable identity
// lifecycle in lockstep. Legacy worktrees have no identity ID and retain the
// pre-identity behavior.
func removePhysicalAndIdentity(_ *cobra.Command, wm vcs.Manager, repoDir, ref, cwd string, wt vcs.Worktree) error {
	pathMissing := false
	if wt.WorkspaceID != "" {
		_, err := os.Lstat(wt.Path)
		pathMissing = os.IsNotExist(err)
	}
	if wt.WorkspaceID != "" {
		if err := cleanupResources(wt.WorkspaceID, repoDir, wt.Path, wm, func() error {
			store, storeErr := removalIdentityStoreFactory()
			if storeErr != nil {
				return storeErr
			}
			_, markErr := store.MarkCleanupFailed(wt.WorkspaceID)
			return markErr
		}); err != nil {
			return err
		}
	}
	if !pathMissing {
		// The PID file lives inside the workspace, so stop the server before the
		// directory is removed. Keep the port lease until the lifecycle succeeds.
		if wt.Path != "" {
			_ = port.StopDevServer(wt.Path)
		}
		if err := wm.Remove(repoDir, ref, cwd, rmForce); err != nil {
			// Keep identity Active so a transient physical failure can be retried.
			return err
		}
	}
	if wt.WorkspaceID != "" {
		store, err := removalIdentityStoreFactory()
		if err != nil {
			return fmt.Errorf("physically removed %s, but identity cleanup failed for workspace %s; repair identity state before retrying: %w", ref, wt.WorkspaceID, err)
		}
		workspace, lookupErr := store.LookupWorkspace(wt.WorkspaceID)
		if lookupErr != nil {
			return fmt.Errorf("physically removed %s, but identity cleanup failed for workspace %s; repair identity state before retrying: %w", ref, wt.WorkspaceID, lookupErr)
		}
		var finalizeErr error
		if workspace.LifecycleState == identity.CleanupFailed {
			_, finalizeErr = store.FinalizeCleanup(wt.WorkspaceID)
		} else {
			_, finalizeErr = store.RemoveWorkspace(wt.WorkspaceID)
		}
		if finalizeErr != nil {
			failed, markErr := store.MarkCleanupFailed(wt.WorkspaceID)
			if markErr != nil {
				return fmt.Errorf("physically removed %s, but identity cleanup failed for workspace %s and could not record cleanup_failed: %w (recording failure: %v)", ref, wt.WorkspaceID, finalizeErr, markErr)
			}
			return fmt.Errorf("physically removed %s, identity is cleanup_failed for workspace %s: %w", ref, failed.ID, finalizeErr)
		}
	}
	return nil
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
		Branch       string `json:"branch"`
		Repo         string `json:"repo,omitempty"`
		VCS          string `json:"vcs,omitempty"`
		RepositoryID string `json:"repository_id,omitempty"`
		WorkspaceID  string `json:"workspace_id,omitempty"`
		Name         string `json:"name,omitempty"`
		NativeName   string `json:"native_name,omitempty"`
		Error        string `json:"error,omitempty"`
	}

	stderr := cmd.ErrOrStderr()
	var errs []error
	var jsonResults []rmGlobalResult

	// remove runs a single resolved match and records the outcome. Each match
	// carries its own backend, so nothing has to be re-detected here.
	remove := func(m globalMatch, query string) {
		ref := nativeWorktreeRef(m.wt)
		if rmErr := removePhysicalAndIdentity(cmd, m.mgr, m.repo, ref, cwd, m.wt); rmErr != nil {
			_, _ = fmt.Fprintf(stderr, "%s failed to remove %s %s: %s\n",
				redBold("✗"), cyan(query), dim("("+m.label()+")"), friendlyError(rmErr))
			errs = append(errs, fmt.Errorf("removing %q from %s: %w", query, m.label(), rmErr))
			if jsonOutput {
				jsonResults = append(jsonResults, rmGlobalResult{
					Branch: m.wt.Branch, Repo: m.repo, VCS: m.mgr.Kind().Label(),
					RepositoryID: m.wt.RepositoryID, WorkspaceID: m.wt.WorkspaceID,
					Name: m.wt.Name, NativeName: m.wt.NativeName, Error: rmErr.Error(),
				})
			}
			return
		}
		if jsonOutput {
			jsonResults = append(jsonResults, rmGlobalResult{
				Branch: m.wt.Branch, Repo: m.repo, VCS: m.mgr.Kind().Label(),
				RepositoryID: m.wt.RepositoryID, WorkspaceID: m.wt.WorkspaceID,
				Name: m.wt.Name, NativeName: m.wt.NativeName,
			})
			return
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s for %s %s\n",
			greenBold("✔"), m.mgr.Kind().Noun(),
			cyan(m.wt.Branch), dim("("+filepath.Base(m.repo)+")"))
	}

	for _, branch := range branches {
		matches, findErr := findGlobalStrict(cmd, repos, branch)
		if findErr != nil {
			errs = append(errs, findErr)
			continue
		}

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
		return writeJSON(cmd.OutOrStdout(), map[string]any{"version": 1, "removed": jsonResults})
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
		wt, resolveErr := resolveWorktree(dir, item.Branch, wm)
		if resolveErr != nil {
			errs = append(errs, resolveErr)
			continue
		}
		if rmErr := removePhysicalAndIdentity(cmd, wm, dir, nativeWorktreeRef(wt), cwd, wt); rmErr != nil {
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

	groups, err := collectGlobalStrict(cmd, repos)
	if err != nil {
		return err
	}

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
		if rmErr := removePhysicalAndIdentity(cmd, m.mgr, m.repo, nativeWorktreeRef(m.wt), cwd, m.wt); rmErr != nil {
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
