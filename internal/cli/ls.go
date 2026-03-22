package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

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

type lsRow struct {
	branch     string // plain text for width calculation
	path       string
	head       string
	isMain     bool
	isDetached bool
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

	rows := make([]lsRow, len(wts))
	for i, wt := range wts {
		branch := wt.Branch
		if wt.IsMain {
			branch += " *"
		}
		if wt.IsDetached {
			branch = "(detached)"
		}
		rows[i] = lsRow{
			branch:     branch,
			path:       wt.Path,
			head:       shortHead(wt.Head),
			isMain:     wt.IsMain,
			isDetached: wt.IsDetached,
		}
	}

	printWorktreeTable(cmd, rows, "")
	return nil
}

// colWidths holds pre-calculated column widths for consistent alignment.
type colWidths struct {
	branch int
	path   int
}

// calcWidths returns the column widths needed for a set of rows.
func calcWidths(rows []lsRow) colWidths {
	bw, pw := len("BRANCH"), len("PATH")
	for _, r := range rows {
		if len(r.branch) > bw {
			bw = len(r.branch)
		}
		if len(r.path) > pw {
			pw = len(r.path)
		}
	}
	return colWidths{branch: bw, path: pw}
}

// mergeWidths returns the element-wise max of two colWidths.
func mergeWidths(a, b colWidths) colWidths {
	if b.branch > a.branch {
		a.branch = b.branch
	}
	if b.path > a.path {
		a.path = b.path
	}
	return a
}

// printWorktreeTable renders a colored, aligned worktree table.
// prefix is prepended to each line (e.g. "  " for indented global output).
func printWorktreeTable(cmd *cobra.Command, rows []lsRow, prefix string) {
	printWorktreeTableWithWidths(cmd, rows, prefix, calcWidths(rows))
}

// printWorktreeTableWithWidths renders with explicit column widths for cross-table alignment.
func printWorktreeTableWithWidths(cmd *cobra.Command, rows []lsRow, prefix string, w colWidths) {
	out := cmd.OutOrStdout()

	gap := 2
	// Header
	_, _ = fmt.Fprintf(out, "%s%s%s%s\n",
		prefix,
		bold(pad("BRANCH", w.branch+gap)),
		bold(pad("PATH", w.path+gap)),
		bold("HEAD"),
	)

	// Data rows
	for _, r := range rows {
		var coloredBranch string
		switch {
		case r.isMain:
			coloredBranch = green(pad(r.branch, w.branch+gap))
		case r.isDetached:
			coloredBranch = yellow(pad(r.branch, w.branch+gap))
		default:
			coloredBranch = cyan(pad(r.branch, w.branch+gap))
		}
		_, _ = fmt.Fprintf(out, "%s%s%s%s\n",
			prefix,
			coloredBranch,
			pad(r.path, w.path+gap),
			dim(r.head),
		)
	}
}

// pad right-pads s with spaces to width w.
func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// repoEntry represents a repo and its worktrees for JSON output.
type repoEntry struct {
	Repo      string         `json:"repo"`
	Worktrees []git.Worktree `json:"worktrees"`
}

func runLsGlobal(cmd *cobra.Command, wm *git.WorktreeManager) error {
	repos, err := config.LoadValid()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(repos) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("No registered repos. Run a wtf command inside a repo to auto-register it."))
		return nil
	}

	if lsJSON {
		return runLsGlobalJSON(cmd, wm, repos)
	}

	out := cmd.OutOrStdout()

	// First pass: collect all rows per repo and compute global column widths.
	type repoRows struct {
		name string
		path string
		rows []lsRow
	}
	var groups []repoRows
	globalW := colWidths{}

	for _, repo := range repos {
		wts, err := wm.List(repo)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Could not list %s: %v\n", yellow("⚠"), cyan(repo), err)
			continue
		}

		rows := make([]lsRow, len(wts))
		for j, wt := range wts {
			branch := wt.Branch
			if wt.IsMain {
				branch += " *"
			}
			if wt.IsDetached {
				branch = "(detached)"
			}
			rows[j] = lsRow{
				branch:     branch,
				path:       wt.Path,
				head:       shortHead(wt.Head),
				isMain:     wt.IsMain,
				isDetached: wt.IsDetached,
			}
		}
		globalW = mergeWidths(globalW, calcWidths(rows))
		groups = append(groups, repoRows{name: filepath.Base(repo), path: repo, rows: rows})
	}

	// Second pass: print with consistent widths.
	for i, g := range groups {
		_, _ = fmt.Fprintf(out, "%s %s\n", cyanBold(g.name), dim("("+g.path+")"))
		printWorktreeTableWithWidths(cmd, g.rows, "  ", globalW)

		if i < len(groups)-1 {
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
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Could not list %s: %v\n", yellow("⚠"), repo, err)
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
