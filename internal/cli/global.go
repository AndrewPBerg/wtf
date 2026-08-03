package cli

import (
	"fmt"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
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
func collectGlobal(cmd *cobra.Command, repos []string) []globalGroup {
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
			groups = append(groups, globalGroup{repo: repo, mgr: mgr, wts: wts})
		}
	}
	return groups
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
func findGlobal(cmd *cobra.Command, repos []string, query string) []globalMatch {
	var matches []globalMatch
	for _, g := range collectGlobal(cmd, repos) {
		wt, err := g.mgr.Find(g.repo, query)
		if err != nil {
			continue
		}
		matches = append(matches, globalMatch{wt: wt, repo: g.repo, mgr: g.mgr})
	}
	return matches
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
