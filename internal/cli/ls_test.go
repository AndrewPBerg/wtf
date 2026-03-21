package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLsCommand_Table(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = false
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
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = true
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
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = false
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
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = true
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
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = false
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
	cmd := lsCmd
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	lsJSON = false
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
		{branch: "main", path: "/short"},
		{branch: "feature-very-long-branch", path: "/a/very/long/path/here"},
	}
	w := calcWidths(rows)
	assert.Equal(t, len("feature-very-long-branch"), w.branch)
	assert.Equal(t, len("/a/very/long/path/here"), w.path)
}

func TestCalcWidths_EmptyRows(t *testing.T) {
	w := calcWidths(nil)
	// Should default to header widths
	assert.Equal(t, len("BRANCH"), w.branch)
	assert.Equal(t, len("PATH"), w.path)
}

func TestMergeWidths(t *testing.T) {
	a := colWidths{branch: 5, path: 10}
	b := colWidths{branch: 8, path: 3}
	m := mergeWidths(a, b)
	assert.Equal(t, 8, m.branch)
	assert.Equal(t, 10, m.path)
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
	cmd := lsCmd
	cmd.SetOut(buf)
	lsJSON = false
	lsGlobal = false

	err = runLs(cmd, wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "(detached)")
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
