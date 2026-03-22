package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	lsGlobal bool
	lsPRs    bool
)

func init() {
	lsCmd.Flags().BoolVarP(&lsGlobal, "global", "g", false, "List worktrees across all registered repos")
	lsCmd.Flags().BoolVarP(&lsPRs, "prs", "p", false, "Show PR status for each worktree")
	rootCmd.AddCommand(lsCmd)
	lsgCmd.Flags().BoolVarP(&lsPRs, "prs", "p", false, "Show PR status for each worktree")
	rootCmd.AddCommand(lsgCmd)
}

var lsgCmd = &cobra.Command{
	Use:   "lsg",
	Short: "List all worktrees globally (shortcut for ls -g)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		lsGlobal = true
		return runLs(cmd, git.NewWorktreeManager(&git.RealExecutor{}))
	},
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
	commitURL  string // web URL for the commit (empty if unavailable)
	isMain     bool
	isDetached bool
	// PR fields (populated when --prs is used)
	prNumber int
	prTitle  string
	prAuthor string
	prURL    string
	prReview forge.ReviewStatus
	prDraft  bool
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

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(wts)
	}

	// Best-effort remote URL for commit hyperlinks.
	remoteURL, _ := wm.RemoteURL(dir)

	// Two-phase async render: show cached PR data instantly, update when fresh arrives.
	isTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	if lsPRs && isTTY {
		return runLsWithAsyncPRs(cmd, wts, remoteURL, dir)
	}

	// Synchronous path (no --prs, JSON, or piped output).
	var prMap map[string]forge.PR
	if lsPRs {
		prMap = fetchPRMap(cmd, remoteURL, dir)
	}

	rows := buildRows(wts, remoteURL, prMap)
	printWorktreeTable(cmd, rows, "")
	return nil
}

// runLsWithAsyncPRs renders the worktree table with lazy-loaded PR data.
// It immediately displays cached data, then re-renders in-place if the
// fresh API response differs.
func runLsWithAsyncPRs(cmd *cobra.Command, wts []git.Worktree, remoteURL, dir string) error {
	cf := createCachedForge(cmd, remoteURL, dir)
	if cf == nil {
		// No forge available — render without PRs.
		rows := buildRows(wts, remoteURL, nil)
		printWorktreeTable(cmd, rows, "")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch := cf.ListPRsAsync(ctx)
	rr := ui.NewRerenderer(cmd.OutOrStdout())

	var lastMap map[string]forge.PR
	for result := range ch {
		if result.Err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s PR info unavailable: %v\n", yellow("⚠"), result.Err)
			continue
		}

		prMap := prsToBranchMap(result.PRs)

		// Skip re-render if data hasn't changed.
		if lastMap != nil && prMapsEqual(lastMap, prMap) {
			continue
		}
		lastMap = prMap

		rows := buildRows(wts, remoteURL, prMap)
		w := calcWidths(rows)
		content := renderWorktreeTable(rows, "", w, true)
		rr.Render(content)
	}

	// If we never rendered (e.g. all results were errors), render without PRs.
	if lastMap == nil {
		rows := buildRows(wts, remoteURL, nil)
		printWorktreeTable(cmd, rows, "")
	}
	return nil
}

// createCachedForge creates a CachedForge for the given remote URL and directory.
// Returns nil if forge detection fails.
func createCachedForge(cmd *cobra.Command, remoteURL, dir string) *forge.CachedForge {
	if remoteURL == "" {
		return nil
	}

	f, err := forge.Detect(remoteURL, forge.WithTokenFunc(func() (string, error) {
		return tryToken(remoteURL)
	}))
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s PR info unavailable: %v\n", yellow("⚠"), err)
		return nil
	}

	exec := &git.RealExecutor{}
	gitCommonDir, gcErr := exec.Run(dir, "rev-parse", "--git-common-dir")
	if gcErr != nil {
		return nil
	}

	return forge.NewCachedForge(f, gitCommonDir)
}

// prsToBranchMap converts a slice of PRs into a map keyed by branch name.
func prsToBranchMap(prs []forge.PR) map[string]forge.PR {
	m := make(map[string]forge.PR, len(prs))
	for _, pr := range prs {
		m[pr.Branch] = pr
	}
	return m
}

// prMapsEqual returns true if two PR maps have the same display-relevant data.
func prMapsEqual(a, b map[string]forge.PR) bool {
	if len(a) != len(b) {
		return false
	}
	for k, pa := range a {
		pb, ok := b[k]
		if !ok {
			return false
		}
		if pa.Number != pb.Number || pa.Title != pb.Title ||
			pa.ReviewStatus != pb.ReviewStatus || pa.IsDraft != pb.IsDraft {
			return false
		}
	}
	return true
}

// buildRows converts worktrees and optional PR data into display rows.
// PRs that don't match any local worktree are appended as orphan rows.
func buildRows(wts []git.Worktree, remoteURL string, prMap map[string]forge.PR) []lsRow {
	rows := make([]lsRow, len(wts))
	matched := make(map[string]bool, len(wts))
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
			commitURL:  git.CommitURL(remoteURL, wt.Head),
			isMain:     wt.IsMain,
			isDetached: wt.IsDetached,
		}
		if pr, ok := prMap[wt.Branch]; ok {
			rows[i].prNumber = pr.Number
			rows[i].prTitle = pr.Title
			rows[i].prAuthor = pr.Author
			rows[i].prURL = pr.URL
			rows[i].prReview = pr.ReviewStatus
			rows[i].prDraft = pr.IsDraft
			matched[wt.Branch] = true
		}
	}

	// Append orphan PRs — open PRs from other authors/branches with no local worktree.
	for branch, pr := range prMap {
		if matched[branch] {
			continue
		}
		rows = append(rows, lsRow{
			branch:   branch,
			prNumber: pr.Number,
			prTitle:  pr.Title,
			prAuthor: pr.Author,
			prURL:    pr.URL,
			prReview: pr.ReviewStatus,
			prDraft:  pr.IsDraft,
		})
	}

	return rows
}

// fetchPRMap fetches open PRs and returns a map keyed by branch name.
// Used by the synchronous path (piped output, global mode).
func fetchPRMap(cmd *cobra.Command, remoteURL, dir string) map[string]forge.PR {
	if remoteURL == "" {
		return nil
	}

	f, err := forge.Detect(remoteURL, forge.WithTokenFunc(func() (string, error) {
		return tryToken(remoteURL)
	}))
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s PR info unavailable: %v\n", yellow("⚠"), err)
		return nil
	}

	// Try to use cache
	exec := &git.RealExecutor{}
	gitCommonDir, gcErr := exec.Run(dir, "rev-parse", "--git-common-dir")
	if gcErr == nil {
		f = forge.NewCachedForge(f, gitCommonDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prs, err := f.ListPRs(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s PR info unavailable: %v\n", yellow("⚠"), err)
		return nil
	}

	return prsToBranchMap(prs)
}

// tryToken attempts to get a token for the given remote URL.
func tryToken(remoteURL string) (string, error) {
	if strings.Contains(remoteURL, "github") {
		return ghTokenSafe()
	}
	return glabTokenSafe()
}

// ghTokenSafe is a non-fatal version of gh auth token.
func ghTokenSafe() (string, error) {
	out, err := osexec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// glabTokenSafe is a non-fatal version of glab auth token.
func glabTokenSafe() (string, error) {
	out, err := osexec.Command("glab", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// colWidths holds pre-calculated column widths for consistent alignment.
type colWidths struct {
	branch int
	path   int
	head   int
	author int
	pr     int
}

// calcWidths returns the column widths needed for a set of rows.
func calcWidths(rows []lsRow) colWidths {
	bw, pw, hw, aw, prw := len("BRANCH"), len("PATH"), len("HEAD"), len("AUTHOR"), len("PR")
	for _, r := range rows {
		if len(r.branch) > bw {
			bw = len(r.branch)
		}
		if len(r.path) > pw {
			pw = len(r.path)
		}
		if len(r.head) > hw {
			hw = len(r.head)
		}
		if len(r.prAuthor) > aw {
			aw = len(r.prAuthor)
		}
		if r.prNumber > 0 {
			// PR cell: "#N title icon" — estimate visible width
			prText := fmt.Sprintf("#%d %s", r.prNumber, truncate(r.prTitle, 40))
			if len(prText) > prw {
				prw = len(prText)
			}
		}
	}
	return colWidths{branch: bw, path: pw, head: hw, author: aw, pr: prw}
}

// mergeWidths returns the element-wise max of two colWidths.
func mergeWidths(a, b colWidths) colWidths {
	if b.branch > a.branch {
		a.branch = b.branch
	}
	if b.path > a.path {
		a.path = b.path
	}
	if b.head > a.head {
		a.head = b.head
	}
	if b.author > a.author {
		a.author = b.author
	}
	if b.pr > a.pr {
		a.pr = b.pr
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
	_, _ = fmt.Fprint(cmd.OutOrStdout(), renderWorktreeTable(rows, prefix, w, lsPRs))
}

// renderWorktreeTable builds the formatted worktree table as a string.
func renderWorktreeTable(rows []lsRow, prefix string, w colWidths, hasPRs bool) string {
	var sb strings.Builder

	gap := 2
	// Header — when PRs are shown, column order: BRANCH HEAD AUTHOR PR PATH
	// Without PRs: BRANCH PATH HEAD
	if hasPRs {
		header := fmt.Sprintf("%s%s%s%s%s%s",
			prefix,
			bold(pad("BRANCH", w.branch+gap)),
			bold(pad("HEAD", w.head+gap)),
			bold(pad("AUTHOR", w.author+gap)),
			bold(pad("PR", w.pr+gap)),
			bold("PATH"),
		)
		sb.WriteString(header)
	} else {
		header := fmt.Sprintf("%s%s%s%s",
			prefix,
			bold(pad("BRANCH", w.branch+gap)),
			bold(pad("PATH", w.path+gap)),
			bold("HEAD"),
		)
		sb.WriteString(header)
	}
	sb.WriteByte('\n')

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

		if hasPRs {
			headStr := dim(pad(r.head, w.head+gap))
			if r.commitURL != "" {
				headStr = hyperlink(r.commitURL, dim(pad(r.head, w.head+gap)))
			}

			authorStr := pad(r.prAuthor, w.author+gap)

			prCell := pad("", w.pr+gap)
			if r.prNumber > 0 {
				prLabel := fmt.Sprintf("#%d", r.prNumber)
				prLink := hyperlink(r.prURL, cyan(prLabel))
				title := truncate(r.prTitle, 40)
				reviewIcon := reviewStatusIcon(r.prReview)
				if r.prDraft {
					reviewIcon = dim("draft")
				}
				prCell = fmt.Sprintf("%s %s %s", prLink, dim(title), reviewIcon)
			}

			line := fmt.Sprintf("%s%s%s%s%s%s",
				prefix,
				coloredBranch,
				headStr,
				authorStr,
				prCell,
				r.path,
			)
			sb.WriteString(line)
		} else {
			headStr := dim(r.head)
			if r.commitURL != "" {
				headStr = hyperlink(r.commitURL, dim(r.head))
			}

			line := fmt.Sprintf("%s%s%s%s",
				prefix,
				coloredBranch,
				pad(r.path, w.path+gap),
				headStr,
			)
			sb.WriteString(line)
		}

		sb.WriteByte('\n')
	}

	return sb.String()
}

// truncate shortens a string to maxLen, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// reviewStatusIcon returns a colored icon for the review status.
func reviewStatusIcon(status forge.ReviewStatus) string {
	switch status {
	case forge.ReviewApproved:
		return green("✔")
	case forge.ReviewChanges:
		return yellow("✖")
	case forge.ReviewPending:
		return dim("○")
	default:
		return ""
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

	if jsonOutput {
		return runLsGlobalJSON(cmd, wm, repos)
	}

	out := cmd.OutOrStdout()

	// Detect current repo so we can highlight it.
	currentRepo, _ := getRepoDir()

	// First pass: collect all rows per repo and compute global column widths.
	type repoRows struct {
		name      string
		path      string
		rows      []lsRow
		isCurrent bool
	}
	var groups []repoRows
	globalW := colWidths{}

	for _, repo := range repos {
		wts, err := wm.List(repo)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Could not list %s: %v\n", yellow("⚠"), cyan(repo), err)
			continue
		}

		// Best-effort remote URL for commit hyperlinks.
		remoteURL, _ := wm.RemoteURL(repo)

		// Best-effort PR lookup when --prs is set.
		var prMap map[string]forge.PR
		if lsPRs {
			prMap = fetchPRMap(cmd, remoteURL, repo)
		}

		rows := buildRows(wts, remoteURL, prMap)
		globalW = mergeWidths(globalW, calcWidths(rows))
		groups = append(groups, repoRows{name: filepath.Base(repo), path: repo, rows: rows, isCurrent: repo == currentRepo})
	}

	// Second pass: print with consistent widths.
	for i, g := range groups {
		if g.isCurrent {
			_, _ = fmt.Fprintf(out, "%s %s %s\n", green("▸"), cyanBold(g.name), dim("("+g.path+")"))
		} else {
			_, _ = fmt.Fprintf(out, "  %s %s\n", cyanBold(g.name), dim("("+g.path+")"))
		}
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
