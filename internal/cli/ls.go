package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

var lsJSON bool

func init() {
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "Output in JSON format")
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

func shortHead(head string) string {
	if len(head) > 7 {
		return head[:7]
	}
	return head
}
