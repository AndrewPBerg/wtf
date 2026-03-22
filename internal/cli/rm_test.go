package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRmCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "to-delete", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(buf)
	rmForce = false

	err = runRm(cmd, "to-delete", wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Removed worktree for to-delete")

	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 1)
}

func TestRmCommand_NonexistentBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	buf := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(buf)

	err := runRm(cmd, "does-not-exist", wm)
	assert.Error(t, err)
}

func TestRmCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	buf := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(buf)

	err := runRm(cmd, "some-branch", wm)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestRmCommand_MainWorktree(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	buf := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(buf)

	err := runRm(cmd, "main", wm)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrMainWorktree)
}
