package cli

import (
	"fmt"
	"os"

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
	Use:               "clean",
	Short:             "Remove worktrees for merged or prunable branches",
	ValidArgsFunction: completeCleanTargets,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runClean(cmd, git.NewWorktreeManager(&git.RealExecutor{}), &git.RealExecutor{})
	},
}

func runClean(cmd *cobra.Command, wm *git.WorktreeManager, exec git.Executor) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
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
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{
				"dry_run": cleanDryRun,
				"removed": []any{},
			})
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("Nothing to clean"))
		return nil
	}

	type cleanResult struct {
		Branch string `json:"branch"`
		Reason string `json:"reason"`
		Error  string `json:"error,omitempty"`
	}
	var jsonResults []cleanResult

	for _, wt := range toRemove {
		reason := "merged"
		if wt.Prunable {
			reason = "prunable"
		}

		if cleanDryRun {
			if jsonOutput {
				jsonResults = append(jsonResults, cleanResult{Branch: wt.Branch, Reason: reason})
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Would remove %s %s\n", yellow("~"), cyan(wt.Branch), dim("("+reason+")"))
			}
			continue
		}

		if err := wm.Remove(dir, wt.Branch, cwd, cleanForce); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Could not remove %s: %v\n", yellow("⚠"), cyan(wt.Branch), err)
			if jsonOutput {
				jsonResults = append(jsonResults, cleanResult{Branch: wt.Branch, Reason: reason, Error: err.Error()})
			}
			continue
		}
		if jsonOutput {
			jsonResults = append(jsonResults, cleanResult{Branch: wt.Branch, Reason: reason})
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Removed %s %s\n", greenBold("✔"), cyan(wt.Branch), dim("("+reason+")"))
		}
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"dry_run": cleanDryRun,
			"removed": jsonResults,
		})
	}

	return nil
}
