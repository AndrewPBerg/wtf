package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

// globalGroup is one repo/backend pairing and the checkouts it contains.
//
// The pairing is the unit rather than the repo alone because a colocated repo the
// user has not decided about contributes two groups — its git worktrees and its
// jj workspaces are different directories and both genuinely exist.
type globalGroup struct {
	repo string
	mgr  vcs.Manager
	wts  []vcs.Worktree
}

// kind returns the backend label for the group.
func (g globalGroup) kind() vcs.Kind { return g.mgr.Kind() }

// collectGlobal lists every checkout across all registered repos.
//
// Nothing here prompts: global commands must never stop to ask which backend a
// repo means, so ambiguity is resolved by listing both and letting each row
// carry its own kind.
func collectGlobalStrict(cmd *cobra.Command, repos []string) ([]globalGroup, error) {
	var groups []globalGroup
	for _, repo := range repos {
		mgrs := managersForRepo(repo)
		if len(mgrs) == 0 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"%s Could not determine the backend for %s\n", yellow("⚠"), cyan(repo))
			continue
		}
		for _, mgr := range mgrs {
			wts, err := mgr.List(repo)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Could not list %s (%s): %v\n",
					yellow("⚠"), cyan(repo), mgr.Kind().Label(), err)
				continue
			}
			wts, err = enrichWorktrees(mgr, repo, wts)
			if err != nil {
				return nil, fmt.Errorf("enriching %s (%s): %w", repo, mgr.Kind().Label(), err)
			}
			groups = append(groups, globalGroup{repo: repo, mgr: mgr, wts: wts})
		}
	}
	return groups, nil
}

// loadGlobalRepos returns the registered repos, or an error when none are known.
func loadGlobalRepos() ([]string, error) {
	repos, err := config.LoadValid()
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no registered repos — run a wtf command inside a repo to auto-register it")
	}
	return repos, nil
}

// globalMatch is a single checkout resolved by a global query, tagged with where
// it came from so the caller can act on it without re-detecting anything.
type globalMatch struct {
	wt   vcs.Worktree
	repo string
	mgr  vcs.Manager
}

// label renders the match's origin for display, e.g. "myrepo · jj".
func (m globalMatch) label() string {
	return filepath.Base(m.repo) + " · " + m.mgr.Kind().Label()
}

// findGlobal resolves a query against every repo and backend.
func findGlobalStrict(cmd *cobra.Command, repos []string, query string) ([]globalMatch, error) {
	var matches []globalMatch
	groups, err := collectGlobalStrict(cmd, repos)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		var wt vcs.Worktree
		if identity.ValidateID(query) == nil {
			wt, err = vcs.FindWorkspaceByID(g.wts, query)
		} else {
			var found vcs.Worktree
			found, err = g.mgr.Find(g.repo, query)
			if err == nil {
				matched := false
				for _, enriched := range g.wts {
					if enriched.Path == found.Path {
						wt = enriched
						matched = true
						break
					}
				}
				if !matched {
					err = fmt.Errorf("resolved worktree %q was not present in enriched listing", query)
				}
			}
		}
		if err == nil {
			matches = append(matches, globalMatch{wt: wt, repo: g.repo, mgr: g.mgr})
		}
	}
	if len(matches) == 0 && identity.ValidateID(query) == nil {
		// A UUID can repair a physical-success/tombstone-write-failure even when
		// both backend listings have lost the checkout.
		store, storeErr := removalIdentityStoreFactory()
		if storeErr != nil {
			return nil, storeErr
		}
		workspace, lookupErr := store.LookupWorkspace(query)
		if lookupErr != nil || workspace.LifecycleState != identity.Active {
			return matches, nil
		}
		canonical, pathErr := identity.CanonicalPhysicalPath(workspace.Path)
		if pathErr != nil {
			return nil, pathErr
		}
		if _, statErr := os.Lstat(canonical); statErr == nil {
			return nil, fmt.Errorf("cannot repair workspace %s: physical path %s still exists but no VCS listing can resolve it; repair the VCS registration, then retry", query, canonical)
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("checking physical path for workspace %s: %w", query, statErr)
		}
		for _, repo := range repos {
			registered, repoErr := store.LookupRepository(repo)
			if repoErr != nil || registered.ID != workspace.RepositoryID {
				continue
			}
			for _, mgr := range managersForRepo(repo) {
				if workspace.Backend != string(mgr.Kind()) {
					continue
				}
				matches = append(matches, globalMatch{repo: repo, mgr: mgr, wt: vcs.Worktree{
					Path: canonical, RepositoryID: workspace.RepositoryID, WorkspaceID: workspace.ID,
					Name: workspace.Name, NativeName: workspace.NativeName, Branch: workspace.NativeName, VCS: mgr.Kind(),
				}})
			}
		}
	}
	return matches, nil
}

// globalPickerItems flattens groups into picker items, tagging each with its repo
// and backend. keep decides which checkouts are eligible.
func globalPickerItems(groups []globalGroup, keep func(globalGroup, vcs.Worktree) bool) ([]ui.PickerItem, map[string]globalMatch) {
	var items []ui.PickerItem
	// Paths are unique across every repo and backend, so they key the lookup that
	// carries each row back to the manager that produced it.
	origin := make(map[string]globalMatch)

	for _, g := range groups {
		for _, wt := range g.wts {
			if !keep(g, wt) {
				continue
			}
			items = append(items, ui.PickerItem{
				Branch: wt.Branch,
				Path:   wt.Path,
				Head:   wt.Head,
				IsMain: wt.IsMain,
				Repo:   g.repo,
				VCS:    g.kind().Label(),
			})
			origin[wt.Path] = globalMatch{wt: wt, repo: g.repo, mgr: g.mgr}
		}
	}
	return items, origin
}
