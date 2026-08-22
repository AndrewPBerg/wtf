package cli

import (
	"bytes"
	"fmt"
	"strings"
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

	err = runRmGlobal(cmd, []string{"global-rm"})
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

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"nonexistent"})
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

	err = runRmGlobal(cmd, []string{"dup-rm"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple global matches")
	assert.Contains(t, stderr.String(), "multiple")
}

func TestRmGlobal_NoRepos(t *testing.T) {
	resetRmFlags(t)
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"anything"})
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

	err = runRmGlobal(cmd, []string{"feat-a", "feat-b"})
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

	err = runRmGlobal(cmd, []string{"feat-ok", "nonexistent"})
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

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runRmGlobal(cmd, []string{"main"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "main worktree")
}

// withTTY overrides stdinIsTTY for the duration of the test.
func withTTY(t *testing.T) {
	t.Helper()
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = orig })
}

func TestRmGlobal_MultipleMatches_PromptAll(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)
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
	cmd.SetIn(strings.NewReader("all\n"))

	err = runRmGlobal(cmd, []string{"dup-rm"})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Removed worktree for feature-dup-rm")

	// Both repos should have only main left
	wts1, err := wm.List(repo1)
	require.NoError(t, err)
	assert.Len(t, wts1, 1)

	wts2, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts2, 1)
}

func TestRmGlobal_MultipleMatches_PromptSelectOne(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)
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
	cmd.SetIn(strings.NewReader("1\n"))

	err = runRmGlobal(cmd, []string{"dup-rm"})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "Removed worktree for feature-dup-rm")

	// Only repo1's worktree should be removed
	wts1, err := wm.List(repo1)
	require.NoError(t, err)
	assert.Len(t, wts1, 1) // main only

	wts2, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts2, 2) // main + feature-dup-rm still there
}

func TestRmGlobal_MultipleMatches_PromptNone(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)
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
	cmd.SetIn(strings.NewReader("\n"))

	err = runRmGlobal(cmd, []string{"dup-rm"})
	require.NoError(t, err)

	// Nothing removed
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "skipped")

	wts1, err := wm.List(repo1)
	require.NoError(t, err)
	assert.Len(t, wts1, 2)

	wts2, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts2, 2)
}

func TestRmGlobal_MultipleMatches_PromptCommaSelect(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)
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
	cmd.SetIn(strings.NewReader("1,2\n"))

	err = runRmGlobal(cmd, []string{"dup-rm"})
	require.NoError(t, err)

	// Both should be removed
	wts1, err := wm.List(repo1)
	require.NoError(t, err)
	assert.Len(t, wts1, 1)

	wts2, err := wm.List(repo2)
	require.NoError(t, err)
	assert.Len(t, wts2, 1)
}

func TestRmGlobal_MultipleMatches_InvalidSelection(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)
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
	cmd.SetIn(strings.NewReader("5\n"))

	err = runRmGlobal(cmd, []string{"dup-rm"})
	require.NoError(t, err)

	// Nothing removed on invalid input
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid selection")
}

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"has changes", git.ErrWorktreeHasChanges, "has uncommitted changes — use --force to remove anyway"},
		{"main worktree", git.ErrMainWorktree, "cannot remove main worktree"},
		{"current dir", git.ErrWorktreeIsCurrentDir, "cannot remove worktree you are currently inside"},
		{"generic error", fmt.Errorf("some error"), "some error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, friendlyError(tt.err))
		})
	}
}
