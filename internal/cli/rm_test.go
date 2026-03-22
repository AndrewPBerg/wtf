package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetRmFlags resets package-level rm flags between tests.
func resetRmFlags(t *testing.T) {
	t.Helper()
	rmForce = false
	rmGlobal = false
}

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

func TestRmCommand_MultipleBranches(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "del-a", "main")
	require.NoError(t, err)
	_, err = wm.Add(dir, "del-b", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(buf)
	rmForce = false

	for _, branch := range []string{"del-a", "del-b"} {
		err = runRm(cmd, branch, wm)
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Removed worktree for del-a")
	assert.Contains(t, output, "Removed worktree for del-b")

	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 1) // only main remains
}

func TestRmGlobal_RemovesWorktreeAcrossRepos(t *testing.T) {
	resetRmFlags(t)
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	t.Chdir(repo1)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo2, "feature-global-rm", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runRmGlobal(cmd, []string{"global-rm"}, wm)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Removed worktree for feature-global-rm")

	// Verify worktree was actually removed
	wts, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts, 1) // only main remains
}

func TestRmGlobal_NoMatch(t *testing.T) {
	resetRmFlags(t)
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"nonexistent"}, wm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no global worktree found")
	assert.Contains(t, stderr.String(), "error:")
}

func TestRmGlobal_MultipleMatches(t *testing.T) {
	resetRmFlags(t)
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	t.Chdir(repo1)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo1, "feature-dup-rm", "main")
	require.NoError(t, err)
	_, err = wm.Add(repo2, "feature-dup-rm", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runRmGlobal(cmd, []string{"dup-rm"}, wm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple global matches")
	assert.Contains(t, stderr.String(), "multiple")
}

func TestRmGlobal_NoRepos(t *testing.T) {
	resetRmFlags(t)
	setupGlobalRegistry(t, []string{})

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"anything"}, wm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no registered repos")
}

func TestRmGlobal_MultipleBranches(t *testing.T) {
	resetRmFlags(t)
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	t.Chdir(repo1)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo1, "feat-a", "main")
	require.NoError(t, err)
	_, err = wm.Add(repo2, "feat-b", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runRmGlobal(cmd, []string{"feat-a", "feat-b"}, wm)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Removed worktree for feat-a")
	assert.Contains(t, output, "Removed worktree for feat-b")

	wts1, err := wm.List(repo1)
	require.NoError(t, err)
	assert.Len(t, wts1, 1) // only main

	wts2, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts2, 1) // only main
}

func TestRmGlobal_MultipleBranches_PartialFailure(t *testing.T) {
	resetRmFlags(t)
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo, "feat-ok", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runRmGlobal(cmd, []string{"feat-ok", "nonexistent"}, wm)
	assert.Error(t, err)

	// The successful one should still have been removed
	assert.Contains(t, stdout.String(), "Removed worktree for feat-ok")
	// The failed one should be reported
	assert.Contains(t, stderr.String(), "nonexistent")
}

func TestRmGlobal_MainWorktreeProtection(t *testing.T) {
	resetRmFlags(t)
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"main"}, wm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "main worktree")
}
