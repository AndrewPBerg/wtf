package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

// filterPrefix returns only entries that start with the given prefix.
// If prefix is empty, all entries are returned.
func filterPrefix(items []string, prefix string) []string {
	if prefix == "" {
		return items
	}
	var filtered []string
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// completeWorktrees provides tab-completion with active worktree branch names.
// Used by sw, swg, rm, and rmg commands.
func completeWorktrees(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	dir, err := getRepoDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	wts, err := wm.List(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, wt := range wts {
		if wt.Branch != "" && !wt.IsBare {
			completions = append(completions, wt.Branch)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeRemoteBranches provides tab-completion with remote branches
// that don't already have a local worktree.
func completeRemoteBranches(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)
	bm := git.NewBranchManager(exec)

	dir, err := getRepoDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	remote, err := bm.RemoteBranches(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Build set of branches that already have worktrees
	wts, err := wm.List(dir)
	if err != nil {
		return filterPrefix(remote, toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	existing := make(map[string]bool, len(wts))
	for _, wt := range wts {
		if wt.Branch != "" {
			existing[wt.Branch] = true
		}
	}

	var completions []string
	for _, b := range remote {
		if !existing[b] {
			completions = append(completions, b)
		}
	}

	return filterPrefix(completions, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeCleanTargets provides tab-completion with worktrees that would
// be removed by `wtf clean` (merged or prunable).
func completeCleanTargets(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)
	bm := git.NewBranchManager(exec)

	dir, err := getRepoDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	wts, err := wm.List(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Find the main branch
	mainBranch := "main"
	for _, wt := range wts {
		if wt.IsMain {
			mainBranch = wt.Branch
			break
		}
	}

	merged, err := bm.MergedBranches(dir, mainBranch)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	mergedSet := make(map[string]bool, len(merged))
	for _, b := range merged {
		mergedSet[b] = true
	}

	var completions []string
	for _, wt := range wts {
		if wt.IsMain {
			continue
		}
		if wt.Prunable {
			completions = append(completions, fmt.Sprintf("%s\tprunable", wt.Branch))
		} else if mergedSet[wt.Branch] {
			completions = append(completions, fmt.Sprintf("%s\tmerged", wt.Branch))
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeRemoteBranchValues provides tab-completion for --branch flag values.
func completeRemoteBranchValues(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return completeRemoteBranches(nil, nil, "")
}

// completeRegisteredRepos provides tab-completion with registered repo paths from the registry.
func completeRegisteredRepos(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	paths, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return paths, cobra.ShellCompDirectiveNoFileComp
}

// completePRValues provides tab-completion for --pr flag values.
func completePRValues(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	dir, err := getRepoDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	remoteURL, err := wm.RemoteURL(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	f, err := forge.Detect(remoteURL)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Try to use cache for fast completions
	gitCommonDir, gcErr := exec.Run(dir, "rev-parse", "--git-common-dir")
	if gcErr == nil {
		f = forge.NewCachedForge(f, gitCommonDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prs, err := f.ListPRs(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, pr := range prs {
		desc := fmt.Sprintf("%d\t#%d %s (%s)", pr.Number, pr.Number, pr.Branch, pr.Author)
		completions = append(completions, desc)
	}

	return filterPrefix(completions, toComplete), cobra.ShellCompDirectiveNoFileComp
}
