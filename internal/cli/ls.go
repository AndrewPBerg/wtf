package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var (
	lsJSON   bool
	lsGlobal bool
)

func init() {
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "Output in JSON format")
	lsCmd.Flags().BoolVarP(&lsGlobal, "global", "g", false, "List worktrees across all registered repos")
	rootCmd.AddCommand(lsCmd)
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all worktrees",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runLs(cmd, git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

func runLs(cmd *cobra.Command, wm *git.WorktreeManager) error {
	if lsGlobal {
		return runLsGlobal(cmd, wm)
	}

	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wts, err := wm.List(dir)
	if err != nil {
		return err
	}

	if lsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(wts)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "BRANCH\tPATH\tHEAD\n")
	for _, wt := range wts {
		branch := wt.Branch
		if wt.IsMain {
			branch += " *"
		}
		if wt.IsDetached {
			branch = "(detached)"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", branch, wt.Path, shortHead(wt.Head))
	}
	return w.Flush()
}

// repoEntry represents a repo and its worktrees for JSON output.
type repoEntry struct {
	Repo      string         `json:"repo"`
	Worktrees []git.Worktree `json:"worktrees"`
}

func runLsGlobal(cmd *cobra.Command, wm *git.WorktreeManager) error {
	repos, err := config.Prune()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(repos) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No registered repos. Run a wtf command inside a repo to auto-register it.")
		return nil
	}

	if lsJSON {
		return runLsGlobalJSON(cmd, wm, repos)
	}

	out := cmd.OutOrStdout()
	for i, repo := range repos {
		wts, err := wm.List(repo)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not list %s: %v\n", repo, err)
			continue
		}

		name := filepath.Base(repo)
		_, _ = fmt.Fprintf(out, "%s (%s)\n", name, repo)

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "  BRANCH\tPATH\tHEAD\n")
		for _, wt := range wts {
			branch := wt.Branch
			if wt.IsMain {
				branch += " *"
			}
			if wt.IsDetached {
				branch = "(detached)"
			}
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\n", branch, wt.Path, shortHead(wt.Head))
		}
		_ = w.Flush()

		if i < len(repos)-1 {
			_, _ = fmt.Fprintln(out)
		}
	}
	return nil
}

func runLsGlobalJSON(cmd *cobra.Command, wm *git.WorktreeManager, repos []string) error {
	var entries []repoEntry
	for _, repo := range repos {
		wts, err := wm.List(repo)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not list %s: %v\n", repo, err)
			continue
		}
		entries = append(entries, repoEntry{Repo: repo, Worktrees: wts})
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func shortHead(head string) string {
	if len(head) > 7 {
		return head[:7]
	}
	return head
}
