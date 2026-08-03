package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func TestLsCommand_Table(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = false

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "main *")
}

func TestLsCommand_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = true
	lsGlobal = false

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"branch": "main"`)
	assert.Contains(t, output, `"is_main": true`)
}

func TestLsCommand_Global_Table(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)

	require.NoError(t, config.Add(repo1))
	require.NoError(t, config.Add(repo2))

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, repo1)
	assert.Contains(t, output, repo2)
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "main *")
}

func TestLsCommand_Global_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo1 := initCLITestRepo(t)
	require.NoError(t, config.Add(repo1))

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = true
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"repo"`)
	assert.Contains(t, output, `"worktrees"`)
	assert.Contains(t, output, `"branch": "main"`)
}

func TestLsCommand_Global_NoRepos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "No registered repos")
}

func TestLsCommand_Global_StaleRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo := initCLITestRepo(t)
	require.NoError(t, config.Save([]string{repo, "/nonexistent/repo"}))

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	// Stale repo should be pruned, only valid repo shown
	output := buf.String()
	assert.Contains(t, output, repo)
	assert.NotContains(t, output, "/nonexistent/repo")
}

func TestPad(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"abc", 6, "abc   "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"}, // len >= width, no padding
		{"", 3, "   "},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, pad(tt.input, tt.width))
	}
}

func TestCalcWidths(t *testing.T) {
	rows := []lsRow{
		{branch: "main", path: "/short", head: "abc1234"},
		{branch: "feature-very-long-branch", path: "/a/very/long/path/here", head: "def5678"},
	}
	w := calcWidths(rows, vcs.KindGit)
	assert.Equal(t, len("feature-very-long-branch"), w.branch)
	assert.Equal(t, len("/a/very/long/path/here"), w.path)
	assert.Equal(t, len("abc1234"), w.head) // all heads same length, matches max
}

func TestCalcWidths_EmptyRows(t *testing.T) {
	w := calcWidths(nil, vcs.KindGit)
	// Should default to header widths
	assert.Equal(t, len("BRANCH"), w.branch)
	assert.Equal(t, len("PATH"), w.path)
	assert.Equal(t, len("HEAD"), w.head)
}

func TestMergeWidths(t *testing.T) {
	a := colWidths{branch: 5, path: 10, head: 7}
	b := colWidths{branch: 8, path: 3, head: 4}
	m := mergeWidths(a, b)
	assert.Equal(t, 8, m.branch)
	assert.Equal(t, 10, m.path)
	assert.Equal(t, 7, m.head)
}

func TestLsCommand_WithDetachedHead(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a worktree, then detach HEAD in it
	wtPath, err := wm.Add(dir, "detach-test", "main")
	require.NoError(t, err)

	// Get the HEAD commit and detach
	head, err := exec.Run(wtPath, "rev-parse", "HEAD")
	require.NoError(t, err)
	_, err = exec.Run(wtPath, "checkout", "--detach", head)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = false

	err = runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "(detached)")
}

func TestHyperlink(t *testing.T) {
	result := hyperlink("https://example.com", "click me")
	assert.Contains(t, result, "https://example.com")
	assert.Contains(t, result, "click me")
	// Verify OSC 8 escape sequences
	assert.Contains(t, result, "\033]8;;")
	assert.Contains(t, result, "\033\\")
}

func TestPrintWorktreeTableWithWidths_CommitURL(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)

	rows := []lsRow{
		{
			branch:    "feature-x",
			path:      "/tmp/test",
			head:      "abc1234",
			commitURL: "https://github.com/user/repo/commit/abc1234",
			isMain:    false,
		},
	}
	w := calcWidths(rows, vcs.KindGit)
	printWorktreeTableWithWidths(cmd, rows, "", w, vcs.KindGit)

	output := buf.String()
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "abc1234")
	// Should contain the hyperlink escape
	assert.Contains(t, output, "\033]8;;https://github.com/user/repo/commit/abc1234")
}

func TestLsCommand_Global_Table_CurrentRepoHighlighted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo := initCLITestRepo(t)
	// chdir into the repo so getRepoDir() matches
	t.Chdir(repo)

	require.NoError(t, config.Add(repo))

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	// Current repo gets the "▸" marker
	assert.Contains(t, output, "▸")
}

func TestLsCommand_Global_Table_NonCurrentRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo := initCLITestRepo(t)
	// chdir to a different directory so the repo is NOT current
	t.Chdir(t.TempDir())

	require.NoError(t, config.Add(repo))

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	// Non-current repo should NOT have the "▸" marker
	assert.NotContains(t, output, "▸")
	assert.Contains(t, output, "BRANCH")
}

func TestLsCommand_WithRemoteURL(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)
	wm := git.NewWorktreeManager(&git.RealExecutor{})

	// Add a fake remote to trigger the commitURL path
	exec := &git.RealExecutor{}
	_, err := exec.Run(dir, "remote", "add", "origin", "https://github.com/user/repo.git")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = false

	err = runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "BRANCH")
	// Should contain hyperlink escape since remote URL is set
	assert.Contains(t, output, "\033]8;;")
}

func TestLsCommand_Global_MultipleRepos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)

	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)

	require.NoError(t, config.Add(repo1))
	require.NoError(t, config.Add(repo2))

	// chdir to repo1 to test current repo highlighting
	t.Chdir(repo1)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	jsonOutput = false
	lsGlobal = true

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	// Current repo (repo1) should have "▸", repo2 should not
	assert.Contains(t, output, "▸")
	assert.Contains(t, output, "BRANCH")
}

func TestShortHead(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abc1234567890", "abc1234"},
		{"short", "short"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, shortHead(tt.input))
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a very long string", 10, "this is a…"},
		{"ab", 1, "…"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, truncate(tt.input, tt.maxLen))
	}
}

func TestReviewStatusIcon(t *testing.T) {
	tests := []struct {
		status forge.ReviewStatus
		want   string
	}{
		{forge.ReviewApproved, green("✔")},
		{forge.ReviewChanges, yellow("✖")},
		{forge.ReviewPending, dim("○")},
		{forge.ReviewNone, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, reviewStatusIcon(tt.status))
	}
}

func TestBuildRows_WithPRMap(t *testing.T) {
	wts := []git.Worktree{
		{Path: "/repo/main", Branch: "main", Head: "abc1234567", IsMain: true},
		{Path: "/repo/feat", Branch: "feat-1", Head: "def5678901"},
	}
	prMap := map[string]forge.PR{
		"feat-1": {
			Number: 42,
			Title:  "Add feature",
			Author: "alice",
			URL:    "https://github.com/user/repo/pull/42",
		},
	}

	rows := buildRows(wts, "https://github.com/user/repo.git", prMap)
	require.Len(t, rows, 2)

	// main has no PR
	assert.Equal(t, 0, rows[0].prNumber)

	// feat-1 has PR
	assert.Equal(t, 42, rows[1].prNumber)
	assert.Equal(t, "Add feature", rows[1].prTitle)
	assert.Equal(t, "alice", rows[1].prAuthor)
}

func TestPrMapsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]forge.PR
		b    map[string]forge.PR
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    map[string]forge.PR{},
			b:    map[string]forge.PR{},
			want: true,
		},
		{
			name: "equal",
			a:    map[string]forge.PR{"feat": {Number: 1, Title: "x", ReviewStatus: forge.ReviewApproved}},
			b:    map[string]forge.PR{"feat": {Number: 1, Title: "x", ReviewStatus: forge.ReviewApproved}},
			want: true,
		},
		{
			name: "different length",
			a:    map[string]forge.PR{"feat": {Number: 1}},
			b:    map[string]forge.PR{},
			want: false,
		},
		{
			name: "different number",
			a:    map[string]forge.PR{"feat": {Number: 1, Title: "x"}},
			b:    map[string]forge.PR{"feat": {Number: 2, Title: "x"}},
			want: false,
		},
		{
			name: "different title",
			a:    map[string]forge.PR{"feat": {Number: 1, Title: "x"}},
			b:    map[string]forge.PR{"feat": {Number: 1, Title: "y"}},
			want: false,
		},
		{
			name: "different review status",
			a:    map[string]forge.PR{"feat": {Number: 1, ReviewStatus: forge.ReviewApproved}},
			b:    map[string]forge.PR{"feat": {Number: 1, ReviewStatus: forge.ReviewPending}},
			want: false,
		},
		{
			name: "different draft",
			a:    map[string]forge.PR{"feat": {Number: 1, IsDraft: false}},
			b:    map[string]forge.PR{"feat": {Number: 1, IsDraft: true}},
			want: false,
		},
		{
			name: "different key",
			a:    map[string]forge.PR{"feat-1": {Number: 1}},
			b:    map[string]forge.PR{"feat-2": {Number: 1}},
			want: false,
		},
		{
			name: "ignores non-display fields",
			a:    map[string]forge.PR{"feat": {Number: 1, Title: "x", Author: "alice"}},
			b:    map[string]forge.PR{"feat": {Number: 1, Title: "x", Author: "bob"}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, prMapsEqual(tt.a, tt.b))
		})
	}
}

func TestPrsToBranchMap(t *testing.T) {
	prs := []forge.PR{
		{Number: 1, Branch: "feat-1"},
		{Number: 2, Branch: "feat-2"},
	}
	m := prsToBranchMap(prs)
	assert.Len(t, m, 2)
	assert.Equal(t, 1, m["feat-1"].Number)
	assert.Equal(t, 2, m["feat-2"].Number)
}

func TestPrsToBranchMap_Empty(t *testing.T) {
	m := prsToBranchMap(nil)
	assert.Empty(t, m)
}

func TestRenderWorktreeTable(t *testing.T) {
	rows := []lsRow{
		{branch: "main *", path: "/repo", head: "abc1234", isMain: true},
		{branch: "feat-1", path: "/repo--feat-1", head: "def5678",
			prNumber: 42, prTitle: "Add feature", prURL: "https://example.com/42"},
	}
	w := calcWidths(rows, vcs.KindGit)

	// Without PRs
	out := renderWorktreeTable(rows, "", w, false, vcs.KindGit)
	assert.Contains(t, out, "BRANCH")
	assert.Contains(t, out, "main *")
	assert.NotContains(t, out, "#42")

	// With PRs
	out = renderWorktreeTable(rows, "", w, true, vcs.KindGit)
	assert.Contains(t, out, "PR")
	assert.Contains(t, out, "#42")
	assert.Contains(t, out, "Add feature")
}

func TestBuildRows_OrphanPRs(t *testing.T) {
	wts := []git.Worktree{
		{Path: "/repo/main", Branch: "main", Head: "abc1234567", IsMain: true},
	}
	prMap := map[string]forge.PR{
		"other-branch": {
			Number: 99,
			Title:  "Someone else's PR",
			Author: "bob",
			URL:    "https://github.com/user/repo/pull/99",
		},
	}

	rows := buildRows(wts, "https://github.com/user/repo.git", prMap)
	require.Len(t, rows, 2)

	// First row is the local worktree
	assert.Equal(t, "main *", rows[0].branch)
	assert.Equal(t, 0, rows[0].prNumber)

	// Second row is the orphan PR
	assert.Equal(t, "other-branch", rows[1].branch)
	assert.Equal(t, 99, rows[1].prNumber)
	assert.Equal(t, "Someone else's PR", rows[1].prTitle)
	assert.Equal(t, "bob", rows[1].prAuthor)
	assert.Empty(t, rows[1].path)
	assert.Empty(t, rows[1].head)
}

func TestBuildRows_NilPRMap(t *testing.T) {
	wts := []git.Worktree{
		{Path: "/repo/main", Branch: "main", Head: "abc1234567", IsMain: true},
	}

	rows := buildRows(wts, "", nil)
	require.Len(t, rows, 1)
	assert.Equal(t, 0, rows[0].prNumber)
}

func TestPrintWorktreeTable_WithPRs(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)

	// Enable PR display
	savedPRs := lsPRs
	lsPRs = true
	defer func() { lsPRs = savedPRs }()

	rows := []lsRow{
		{
			branch:   "feat-1",
			path:     "/tmp/test",
			head:     "abc1234",
			prNumber: 42,
			prTitle:  "Add feature",
			prAuthor: "alice",
			prURL:    "https://github.com/user/repo/pull/42",
		},
	}
	w := calcWidths(rows, vcs.KindGit)
	printWorktreeTableWithWidths(cmd, rows, "", w, vcs.KindGit)

	output := buf.String()
	assert.Contains(t, output, "PR")
	assert.Contains(t, output, "#42")
	assert.Contains(t, output, "Add feature")
}
