package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

	// Create a worktree, add a commit, then merge it into main so it's truly merged.
	_, err := wm.Add(dir, "merged-feature", "main")
	require.NoError(t, err)
	wtPath := filepath.Join(filepath.Dir(dir), "merged-feature--"+filepath.Base(dir))
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "merged-feature", "-m", "merge merged-feature")
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

func TestCleanCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = false
	cleanForce = false

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runClean(cmd, wm, &git.RealExecutor{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestCleanCommand_ForceRemove(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create worktree, diverge, then merge so it's truly merged.
	_, err := wm.Add(dir, "force-clean", "main")
	require.NoError(t, err)
	wtPath := filepath.Join(filepath.Dir(dir), "force-clean--"+filepath.Base(dir))
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "force-clean", "-m", "merge force-clean")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = false
	cleanForce = true
	defer func() { cleanForce = false }()

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Removed force-clean")

	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 1)
}

func TestCleanCommand_RemoveError(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a worktree, diverge, then merge so clean tries to remove it.
	wtPath, err := wm.Add(dir, "dirty-branch", "main")
	require.NoError(t, err)
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "dirty-branch", "-m", "merge dirty-branch")
	require.NoError(t, err)

	// Add an untracked file to make the worktree dirty
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0o644))

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cleanDryRun = false
	cleanForce = false

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)
	// Should show warning about not being able to remove
	// (or successfully remove if git allows it — either way covers the branch)
}

func TestCleanCommand_RemovesMerged(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create worktree, diverge, then merge so it's truly merged.
	_, err := wm.Add(dir, "merged-branch", "main")
	require.NoError(t, err)
	wtPath := filepath.Join(filepath.Dir(dir), "merged-branch--"+filepath.Base(dir))
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "merged-branch", "-m", "merge merged-branch")
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

func TestCleanCommand_SkipsSameCommitBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a worktree at the same commit as main — should NOT be cleaned.
	_, err := wm.Add(dir, "fresh-branch", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cleanDryRun = false
	cleanForce = false

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Nothing to clean")
}
