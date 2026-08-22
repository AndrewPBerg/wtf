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
	"unicode/utf8"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	lsGlobal bool
	lsPRs    bool

	// runPickerFunc is the picker entry point. It is a var so tests can stub it
	// without requiring a real TTY.
	runPickerFunc = ui.RunPicker
)

type lsRow struct {
	branch     string // plain text for width calculation
	path       string
	head       string
	commitURL  string // web URL for the commit (empty if unavailable)
	isMain     bool
	isDetached bool
	// jj-only columns. bookmark is display-only — wtf never creates bookmarks.
	bookmark string
	change   string
	// PR fields (populated when --prs is used)
	prNumber int
	prTitle  string
	prAuthor string
	prURL    string
	prReview forge.ReviewStatus
	prDraft  bool
}

func runLs(cmd *cobra.Command, wm vcs.Manager) error {
	if lsGlobal {
		return runLsGlobal(cmd)
	}

	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	wts, err := wm.List(dir)
	if err != nil {
		return err
	}
	wts, err = enrichWorktrees(wm, dir, wts)
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

	// Interactive picker when a user can interact (stdin is a TTY).
	// The picker renders to stderr so stdout stays clean for path capture
	// by the shell wrapper's $().
	canInteract := stdinIsTTY()
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	if canInteract && !lsPRs {
		return runLsInteractive(cmd, wm, wts, dir)
	}

	// Async PR re-rendering needs stdout to be a real TTY (ANSI cursor control).
	if lsPRs && stdoutTTY {
		return runLsWithAsyncPRs(cmd, wm, wts, remoteURL, dir)
	}

	// Synchronous path (no --prs, JSON, or piped output).
	var prMap map[string]forge.PR
	if lsPRs {
		prMap = fetchPRMap(cmd, wm, remoteURL, dir)
	}

	rows := buildRows(wts, remoteURL, prMap)
	printWorktreeTable(cmd, rows, "", wm.Kind())

	// Point out checkouts managed by the other backend rather than pretending
	// they do not exist — a colocated repo can hold both at once.
	warnOtherBackend(cmd, wm, dir)
	return nil
}

// runLsInteractive launches an interactive worktree picker.
// On selection, it prints the worktree path to stdout (like sw).
func runLsInteractive(cmd *cobra.Command, wm vcs.Manager, wts []vcs.Worktree, dir string) error {
	items := worktreesToPickerItems(wts, "", pickerKindLabel(wm, dir))

	result, err := runPickerFunc(items, false)
	if err != nil {
		return err
	}
	if result.Quit || len(result.Items) == 0 {
		return nil
	}

	selected := result.Items[0]
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), selected.Path)

	cwd, _ := os.Getwd()
	if cwd != "" && isCurrentWorktree(cwd, selected.Path) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "wtf? you are already on %s!\n", cyan(selected.Branch))
		return nil
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Switched to %s\n", selected.Path)
	runOnSwitchHooks(cmd, dir, selected.Branch)
	return nil
}

// worktreesToPickerItems converts worktrees to picker items.
// Bare and detached worktrees are excluded (no branch to switch to).
func worktreesToPickerItems(wts []vcs.Worktree, repo string, kind vcs.Kind) []ui.PickerItem {
	items := make([]ui.PickerItem, 0, len(wts))
	for _, wt := range wts {
		if wt.IsBare || wt.IsDetached || wt.Branch == "" {
			continue
		}
		items = append(items, ui.PickerItem{
			Branch: wt.Branch,
			Path:   wt.Path,
			Head:   wt.Head,
			IsMain: wt.IsMain,
			Repo:   repo,
			VCS:    kind.Label(),
		})
	}
	return items
}

// runLsWithAsyncPRs renders the worktree table with lazy-loaded PR data.
// It immediately displays cached data, then re-renders in-place if the
// fresh API response differs.
func runLsWithAsyncPRs(cmd *cobra.Command, wm vcs.Manager, wts []vcs.Worktree, remoteURL, dir string) error {
	cf := createCachedForge(cmd, wm, remoteURL, dir)
	if cf == nil {
		// No forge available — render without PRs.
		rows := buildRows(wts, remoteURL, nil)
		printWorktreeTable(cmd, rows, "", wm.Kind())
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
		w := calcWidths(rows, wm.Kind())
		content := renderWorktreeTable(rows, "", w, true, wm.Kind())
		rr.Render(content)
	}

	// If we never rendered (e.g. all results were errors), render without PRs.
	if lastMap == nil {
		rows := buildRows(wts, remoteURL, nil)
		printWorktreeTable(cmd, rows, "", wm.Kind())
	}
	return nil
}

// createCachedForge creates a CachedForge for the given remote URL and directory.
// Returns nil if forge detection fails.
func createCachedForge(cmd *cobra.Command, mgr vcs.Manager, remoteURL, dir string) *forge.CachedForge {
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

	stateDir, gcErr := mgr.StateDir(dir)
	if gcErr != nil {
		return nil
	}

	return forge.NewCachedForge(f, stateDir)
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
func buildRows(wts []vcs.Worktree, remoteURL string, prMap map[string]forge.PR) []lsRow {
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
			bookmark:   strings.Join(wt.Bookmarks, ","),
			change:     shortHead(wt.ChangeID),
		}
		if wt.Prunable && wt.Path == "" {
			rows[i].path = dimPlaceholder
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
func fetchPRMap(cmd *cobra.Command, mgr vcs.Manager, remoteURL, dir string) map[string]forge.PR {
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
	if stateDir, gcErr := mgr.StateDir(dir); gcErr == nil {
		f = forge.NewCachedForge(f, stateDir)
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
	branch   int
	path     int
	head     int
	author   int
	pr       int
	bookmark int
	change   int
}

// calcWidths returns the column widths needed for a set of rows.
func calcWidths(rows []lsRow, kind vcs.Kind) colWidths {
	bw, pw, hw, aw, prw := len(refHeader(kind)), len("PATH"), len("HEAD"), len("AUTHOR"), len("PR")

	// The bookmark and change columns only exist for jj, so they contribute no
	// width in a git table.
	bkw, cw := 0, 0
	if kind == vcs.KindJJ {
		bkw, cw = len("BOOKMARK"), len("CHANGE")
	}

	for _, r := range rows {
		if n := utf8.RuneCountInString(r.bookmark); n > bkw {
			bkw = n
		}
		if n := utf8.RuneCountInString(r.change); n > cw {
			cw = n
		}
		if n := utf8.RuneCountInString(r.branch); n > bw {
			bw = n
		}
		if n := utf8.RuneCountInString(r.path); n > pw {
			pw = n
		}
		if n := utf8.RuneCountInString(r.head); n > hw {
			hw = n
		}
		if n := utf8.RuneCountInString(r.prAuthor); n > aw {
			aw = n
		}
		if r.prNumber > 0 {
			// PR cell: "#N title icon" — estimate visible width
			prText := fmt.Sprintf("#%d %s", r.prNumber, truncate(r.prTitle, 40))
			if len(prText) > prw {
				prw = len(prText)
			}
		}
	}
	return colWidths{branch: bw, path: pw, head: hw, author: aw, pr: prw, bookmark: bkw, change: cw}
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
	if b.bookmark > a.bookmark {
		a.bookmark = b.bookmark
	}
	if b.change > a.change {
		a.change = b.change
	}
	return a
}

// printWorktreeTable renders a colored, aligned worktree table.
// prefix is prepended to each line (e.g. "  " for indented global output).
func printWorktreeTable(cmd *cobra.Command, rows []lsRow, prefix string, kind vcs.Kind) {
	printWorktreeTableWithWidths(cmd, rows, prefix, calcWidths(rows, kind), kind)
}

// printWorktreeTableWithWidths renders with explicit column widths for cross-table alignment.
func printWorktreeTableWithWidths(cmd *cobra.Command, rows []lsRow, prefix string, w colWidths, kind vcs.Kind) {
	_, _ = fmt.Fprint(cmd.OutOrStdout(), renderWorktreeTable(rows, prefix, w, lsPRs, kind))
}

// renderWorktreeTable builds the formatted worktree table as a string.
func renderWorktreeTable(rows []lsRow, prefix string, w colWidths, hasPRs bool, kind vcs.Kind) string {
	var sb strings.Builder

	gap := 2

	// jj has no branch-per-checkout: workspaces are named, and bookmarks are
	// separate metadata. Naming the columns after what they actually hold is what
	// keeps a jj listing from reading like a git one.
	if kind == vcs.KindJJ && !hasPRs {
		return renderJJTable(rows, prefix, w, gap)
	}
	// Header — when PRs are shown, column order: BRANCH HEAD AUTHOR PR PATH
	// Without PRs: BRANCH PATH HEAD
	if hasPRs {
		header := fmt.Sprintf("%s%s%s%s%s%s",
			prefix,
			bold(pad(refHeader(kind), w.branch+gap)),
			bold(pad("HEAD", w.head+gap)),
			bold(pad("AUTHOR", w.author+gap)),
			bold(pad("PR", w.pr+gap)),
			bold("PATH"),
		)
		sb.WriteString(header)
	} else {
		header := fmt.Sprintf("%s%s%s%s",
			prefix,
			bold(pad(refHeader(kind), w.branch+gap)),
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

// pad right-pads s with spaces to a display width of w.
//
// Width is counted in runes, not bytes: a multi-byte character such as the em
// dash used for "no bookmark" would otherwise consume three columns worth of
// padding and shift everything after it.
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// repoEntry represents a repo and its worktrees for JSON output.
type repoEntry struct {
	Repo      string         `json:"repo"`
	VCS       vcs.Kind       `json:"vcs"`
	Worktrees []vcs.Worktree `json:"worktrees"`
}

func runLsGlobal(cmd *cobra.Command) error {
	repos, err := config.LoadValid()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(repos) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("No registered repos. Run a wtf command inside a repo to auto-register it."))
		return nil
	}

	if jsonOutput {
		return runLsGlobalJSON(cmd, repos)
	}

	// Interactive picker when stdin is a TTY (non-PR view).
	// We check stdin (not stdout) because the shell wrapper captures stdout via $().
	if stdinIsTTY() && !lsPRs {
		return runLsGlobalInteractive(cmd, repos)
	}

	out := cmd.OutOrStdout()

	// Detect current repo so we can highlight it.
	currentRepo, _ := getRepoDir()

	groups, err := collectGlobalStrict(cmd, repos)
	if err != nil {
		return err
	}

	// First pass: build rows and compute column widths shared across every group,
	// so a git table and a jj table still line up beside each other.
	type renderGroup struct {
		name      string
		path      string
		kind      vcs.Kind
		rows      []lsRow
		isCurrent bool
	}
	var rendered []renderGroup
	globalW := colWidths{}

	for _, g := range groups {
		remoteURL, _ := g.mgr.RemoteURL(g.repo)

		var prMap map[string]forge.PR
		if lsPRs {
			prMap = fetchPRMap(cmd, g.mgr, remoteURL, g.repo)
		}

		rows := buildRows(g.wts, remoteURL, prMap)
		globalW = mergeWidths(globalW, calcWidths(rows, g.kind()))
		rendered = append(rendered, renderGroup{
			name:      filepath.Base(g.repo),
			path:      g.repo,
			kind:      g.kind(),
			rows:      rows,
			isCurrent: g.repo == currentRepo,
		})
	}

	// Second pass: print with consistent widths.
	for i, g := range rendered {
		// The backend is always labeled here: one global listing can mix git
		// repos, jj repos, and colocated repos contributing both.
		badge := dim("(" + g.kind.Label() + ")")
		if g.isCurrent {
			_, _ = fmt.Fprintf(out, "%s %s %s %s\n", green("▸"), cyanBold(g.name), badge, dim("("+g.path+")"))
		} else {
			_, _ = fmt.Fprintf(out, "  %s %s %s\n", cyanBold(g.name), badge, dim("("+g.path+")"))
		}
		printWorktreeTableWithWidths(cmd, g.rows, "  ", globalW, g.kind)

		if i < len(rendered)-1 {
			_, _ = fmt.Fprintln(out)
		}
	}
	return nil
}

// runLsGlobalInteractive launches an interactive picker for all registered repos.
func runLsGlobalInteractive(cmd *cobra.Command, repos []string) error {
	groups, err := collectGlobalStrict(cmd, repos)
	if err != nil {
		return err
	}

	allItems, origin := globalPickerItems(groups, func(_ globalGroup, wt vcs.Worktree) bool {
		return !wt.IsBare && !wt.IsDetached && wt.Branch != ""
	})

	if len(allItems) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("No worktrees found across registered repos."))
		return nil
	}

	result, err := runPickerFunc(allItems, false)
	if err != nil {
		return err
	}
	if result.Quit || len(result.Items) == 0 {
		return nil
	}

	selected := result.Items[0]
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), selected.Path)

	cwd, _ := os.Getwd()
	if cwd != "" && isCurrentWorktree(cwd, selected.Path) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "wtf? you are already on %s!\n", cyan(selected.Branch))
		return nil
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Switched to %s\n", selected.Path)
	if m, ok := origin[selected.Path]; ok {
		runOnSwitchHooks(cmd, m.repo, selected.Branch)
	}
	return nil
}

func runLsGlobalJSON(cmd *cobra.Command, repos []string) error {
	var entries []repoEntry
	groups, err := collectGlobalStrict(cmd, repos)
	if err != nil {
		return err
	}
	for _, g := range groups {
		entries = append(entries, repoEntry{
			Repo:      g.repo,
			VCS:       g.kind(),
			Worktrees: g.wts,
		})
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

// dimPlaceholder stands in for a path that no longer exists on disk.
const dimPlaceholder = "(missing)"

// refHeader names the column holding a checkout's identity: git keys on the
// branch, jj on the workspace name.
func refHeader(kind vcs.Kind) string {
	if kind == vcs.KindJJ {
		return "WORKSPACE"
	}
	return "BRANCH"
}

// renderJJTable renders a jj listing: WORKSPACE, BOOKMARK, PATH, CHANGE.
//
// CHANGE carries jj's change id rather than a commit hash, because that is the
// identifier that stays stable as a change is rewritten.
func renderJJTable(rows []lsRow, prefix string, w colWidths, gap int) string {
	var sb strings.Builder

	_, _ = fmt.Fprintf(&sb, "%s%s%s%s%s",
		prefix,
		bold(pad("WORKSPACE", w.branch+gap)),
		bold(pad("BOOKMARK", w.bookmark+gap)),
		bold(pad("PATH", w.path+gap)),
		bold("CHANGE"),
	)
	sb.WriteByte('\n')

	for _, r := range rows {
		branch := cyan(pad(r.branch, w.branch+gap))
		if r.isMain {
			branch = green(pad(r.branch, w.branch+gap))
		}

		// An em dash reads better than blank for "no bookmark here", which is the
		// normal state — wtf never creates one.
		bookmark := r.bookmark
		if bookmark == "" {
			bookmark = "—"
		}

		_, _ = fmt.Fprintf(&sb, "%s%s%s%s%s",
			prefix,
			branch,
			dim(pad(bookmark, w.bookmark+gap)),
			pad(r.path, w.path+gap),
			dim(r.change),
		)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// warnOtherBackend notes checkouts held by the backend that was not chosen, so a
// colocated repo never silently hides half of its state.
func warnOtherBackend(cmd *cobra.Command, mgr vcs.Manager, dir string) {
	other := vcs.KindGit
	if mgr.Kind() == vcs.KindGit {
		other = vcs.KindJJ
	}

	det, err := vcs.Detect(dir)
	if err != nil || !det.Has(other) || !vcs.Available(other) {
		return
	}

	wts, err := newManager(other).List(dir)
	if err != nil {
		return
	}

	// The primary checkout is shared, so only additional ones are news.
	extra := 0
	for _, wt := range wts {
		if !wt.IsMain {
			extra++
		}
	}
	if extra == 0 {
		return
	}

	verb := "exists"
	if extra != 1 {
		verb = "exist"
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s %d %s %s also %s here — %s\n",
		dim("note:"), extra, other.Label(), pluralize(other.Noun(), extra), verb,
		cyan("wtf sw --vcs "+other.Label()))
}

// pluralize adds an "s" when n is not 1.
func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
