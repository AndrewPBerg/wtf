package cli

import (
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(swCmd)
}

var swCmd = &cobra.Command{
	Use:   "sw <branch>",
	Short: "Switch to a worktree (prints path for cd)",
	Long: `Switch to a worktree by branch name (substring match).
Prints the worktree path to stdout so you can cd to it.

To enable the 'sw' shell function that cds automatically, run:

  wtf setup

Or add this to your shell profile manually:

  eval "$(wtf init)"

See 'wtf init --help' and 'wtf setup --help' for details.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSw(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}))
	},
}

func runSw(cmd *cobra.Command, query string, wm *git.WorktreeManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wt, err := wm.Find(dir, query)
	if err == nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), wt.Path)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Switched to %s\n", wt.Path)
		return nil
	}

	// On error, show colored message with available branches
	stderr := cmd.ErrOrStderr()

	wts, listErr := wm.List(dir)
	if listErr != nil || len(wts) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s\n", redBold("error:"), cyan(query))
		// Return the original error so the shell function gets a non-zero exit
		return err
	}

	// Collect branch names
	var branches []string
	for _, w := range wts {
		if w.Branch != "" && !w.IsBare {
			branches = append(branches, w.Branch)
		}
	}

	similar := fuzzyFilter(branches, query)

	_, _ = fmt.Fprintf(stderr, "%s no worktree found matching %s\n", redBold("error:"), cyan(query))

	if len(similar) > 0 {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", dim("Did you mean?"))
		for _, b := range similar {
			_, _ = fmt.Fprintf(stderr, "  %s %s\n", yellow("→"), cyan(b))
		}
	} else if len(branches) > 0 {
		_, _ = fmt.Fprintf(stderr, "\n%s\n", dim("Available worktrees:"))
		for _, b := range branches {
			_, _ = fmt.Fprintf(stderr, "  %s %s\n", dim("•"), cyan(b))
		}
	}

	// Return the original error for non-zero exit code
	return err
}

// fuzzyFilter returns branches that are similar to the query.
// Queries shorter than 2 characters are too ambiguous for fuzzy matching.
func fuzzyFilter(branches []string, query string) []string {
	if len(query) < 2 {
		return nil
	}
	query = strings.ToLower(query)
	type scored struct {
		branch string
		score  int
	}
	var results []scored

	for _, b := range branches {
		bl := strings.ToLower(b)

		// Exact substring match — already handled by Find, skip
		if strings.Contains(bl, query) {
			continue
		}

		score := fuzzyScore(bl, query)
		if score > 0 {
			results = append(results, scored{b, score})
		}
	}

	// Sort by score descending
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	// Return top 5
	var out []string
	for i, r := range results {
		if i >= 5 {
			break
		}
		out = append(out, r.branch)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// fuzzyScore scores how well query characters appear in order in target.
func fuzzyScore(target, query string) int {
	ti := 0
	matched := 0
	for qi := 0; qi < len(query) && ti < len(target); qi++ {
		for ti < len(target) {
			if target[ti] == query[qi] {
				matched++
				ti++
				break
			}
			ti++
		}
	}

	if matched == 0 {
		return 0
	}

	// Require at least 40% of query chars to match in sequence
	threshold := len(query) * 2 / 5
	if threshold < 1 {
		threshold = 1
	}
	if matched < threshold {
		return 0
	}

	return matched
}
