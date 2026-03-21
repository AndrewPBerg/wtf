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
