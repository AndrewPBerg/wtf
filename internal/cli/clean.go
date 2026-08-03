package cli

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/vcs"
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
		wm, err := resolveManager(cmd)
		if err != nil {
			return err
		}
		return runClean(cmd, wm, &git.RealExecutor{})
	},
}

func runClean(cmd *cobra.Command, wm vcs.Manager, _ git.Executor) error {
	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// What counts as spent work is backend-specific: git looks for merged
	// branches, jj for changes already contained in trunk.
	toRemove, err := wm.Cleanable(dir)
	if err != nil {
		return err
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
