package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

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
func completeRemoteBranches(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
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
		return remote, cobra.ShellCompDirectiveNoFileComp
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

	return completions, cobra.ShellCompDirectiveNoFileComp
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
