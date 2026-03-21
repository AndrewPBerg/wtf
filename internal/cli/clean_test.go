package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanCommand_NothingToClean(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = false
	cleanForce = false

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runClean(cmd, wm, &git.RealExecutor{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Nothing to clean")
}

func TestCleanCommand_DryRun(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a worktree with a branch that's merged (same point as main)
	_, err := wm.Add(dir, "merged-feature", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = true
	cleanForce = false

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Would remove merged-feature")

	// Verify worktree still exists
	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 2)
}

func TestCleanCommand_RemovesMerged(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	_, err := wm.Add(dir, "merged-branch", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = false
	cleanForce = false

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Removed merged-branch")

	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 1)
}
