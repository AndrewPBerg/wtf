package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var (
	cleanDryRun bool
	cleanForce  bool
)

func init() {
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "List worktrees that would be removed without removing them")
	cleanCmd.Flags().BoolVar(&cleanForce, "force", false, "Force remove even with uncommitted changes")
	rootCmd.AddCommand(cleanCmd)
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove worktrees for merged or prunable branches",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runClean(cmd, git.NewWorktreeManager(&git.RealExecutor{}), &git.RealExecutor{})
	},
}

func runClean(cmd *cobra.Command, wm *git.WorktreeManager, exec git.Executor) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wts, err := wm.List(dir)
	if err != nil {
		return err
	}

	bm := git.NewBranchManager(exec)
	mainBranch := "main"
	for _, wt := range wts {
		if wt.IsMain {
			mainBranch = wt.Branch
			break
		}
	}

	merged, err := bm.MergedBranches(dir, mainBranch)
	if err != nil {
		return err
	}

	mergedSet := make(map[string]bool)
	for _, b := range merged {
		mergedSet[b] = true
	}

	var toRemove []git.Worktree
	for _, wt := range wts {
		if wt.IsMain {
			continue
		}
		if wt.Prunable || mergedSet[wt.Branch] {
			toRemove = append(toRemove, wt)
		}
	}

	if len(toRemove) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Nothing to clean")
		return nil
	}

	for _, wt := range toRemove {
		reason := "merged"
		if wt.Prunable {
			reason = "prunable"
		}

		if cleanDryRun {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Would remove %s (%s)\n", wt.Branch, reason)
			continue
		}

		if err := wm.Remove(dir, wt.Branch, cleanForce); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not remove %s: %v\n", wt.Branch, err)
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s (%s)\n", wt.Branch, reason)
	}

	return nil
}
