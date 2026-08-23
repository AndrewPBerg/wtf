package jj

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gitDiffTestGitExecutor struct {
	output string
	err    error
}

func (e gitDiffTestGitExecutor) Run(_ string, _ ...string) (string, error) {
	return e.output, e.err
}

type gitDiffTestExecutor struct {
	output string
	err    error
}

func (e gitDiffTestExecutor) Run(_ string, _ ...string) (string, error) {
	return e.output, e.err
}

func TestGitDiffBaseCommitUsesParentAndVirtualRoot(t *testing.T) {
	root := newTestRepo(t)
	manager := NewWorkspaceManager(&RealExecutor{})
	workspace, err := manager.Add(root, "coverage", "main")
	require.NoError(t, err)

	commit, err := manager.GitDiffBaseCommit(workspace)
	require.NoError(t, err)
	assert.NotEmpty(t, commit)

	// A workspace based at the virtual root has no commit baseline.
	base := t.TempDir()
	rootRepo := filepath.Join(base, "root")
	require.NoError(t, os.Mkdir(rootRepo, 0o755))
	runJJ(t, rootRepo, "git", "init", "--colocate")
	rootWorkspace, err := manager.Add(rootRepo, "coverage", "")
	require.NoError(t, err)
	commit, err = manager.GitDiffBaseCommit(rootWorkspace)
	require.NoError(t, err)
	assert.Empty(t, commit)
}

func TestGitDiffBaseSafeErrors(t *testing.T) {
	cases := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "executor error", err: errors.New("jj unavailable"), want: "resolving jj parent for Git diff"},
		{name: "no parent", output: "\n", want: "did not resolve"},
		{name: "multiple parents", output: "one\ntwo\n", want: "exactly one jj parent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewWorkspaceManager(gitDiffTestExecutor{output: tc.output, err: tc.err})
			_, err := manager.GitDiffBaseCommit(t.TempDir())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestInitGitDiffSafeErrors(t *testing.T) {
	root := newTestRepo(t)
	manager := NewWorkspaceManager(&RealExecutor{})
	workspace, err := manager.Add(root, "coverage", "main")
	require.NoError(t, err)

	tests := []struct {
		name string
		git  GitExecutor
		want string
	}{
		{name: "backing git error", git: gitDiffTestGitExecutor{err: errors.New("not readable")}, want: "reading jj Git object format"},
		{name: "unsupported object format", git: gitDiffTestGitExecutor{output: "object"}, want: "unsupported jj Git object format"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager.gitExec = tc.git
			err := manager.InitGitDiff(workspace)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NoDirExists(t, filepath.Join(workspace, ".git"), "failed initialization must clean up")
		})
	}

	assert.Error(t, manager.InitGitDiff(t.TempDir()))
}

func TestRefreshGitDiffSafeErrors(t *testing.T) {
	manager := NewWorkspaceManager(&RealExecutor{})
	workspace := t.TempDir()

	err := manager.RefreshGitDiff(workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no WTF Git diff metadata")

	root := newTestRepo(t)
	workspace, err = manager.Add(root, "coverage", "main")
	require.NoError(t, err)
	require.NoError(t, manager.InitGitDiff(workspace))
	manager.executor = gitDiffTestExecutor{err: errors.New("jj failed")}
	err = manager.RefreshGitDiff(workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving jj parent for Git diff")
}

func TestResetGitDiffBaseUpdatesIndexAndHandlesRoot(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, runPlainGit(repo, "init"))
	require.NoError(t, runPlainGit(repo, "config", "user.name", "wtf test"))
	require.NoError(t, runPlainGit(repo, "config", "user.email", "wtf@example.com"))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file"), []byte("content\n"), 0o644))
	require.NoError(t, runPlainGit(repo, "add", "file"))
	require.NoError(t, runPlainGit(repo, "commit", "-m", "init"))

	commit, err := runWorkspaceGit(repo, "rev-parse", "HEAD")
	require.NoError(t, err)
	require.NoError(t, resetGitDiffBase(repo, gitDiffBase{commit: commit}))
	assert.Equal(t, commit, mustRunWorkspaceGit(t, repo, "rev-parse", "HEAD"))
	assert.Equal(t, "file", mustRunWorkspaceGit(t, repo, "ls-files"))

	require.NoError(t, resetGitDiffBase(repo, gitDiffBase{root: true}))
	assert.Equal(t, "refs/heads/wtf-jj-base", mustRunWorkspaceGit(t, repo, "symbolic-ref", "HEAD"))
	assert.Empty(t, mustRunWorkspaceGit(t, repo, "ls-files"))

	err = resetGitDiffBase(t.TempDir(), gitDiffBase{commit: "deadbeef"})
	assert.Error(t, err)
}

func TestMainWorktreeSuccessAndSafeErrors(t *testing.T) {
	root := newTestRepo(t)
	manager := NewWorkspaceManager(&RealExecutor{})
	workspace, err := manager.Add(root, "coverage", "main")
	require.NoError(t, err)

	main, err := manager.MainWorktree(workspace)
	require.NoError(t, err)
	assert.True(t, main.IsMain)
	assert.Equal(t, root, main.Path)

	manager = NewWorkspaceManager(gitDiffTestExecutor{output: "name\x1f/path\x1fcommit\x1fchange\x1f\n"})
	_, err = manager.MainWorktree(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not identify the main workspace")
}

func mustRunWorkspaceGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runWorkspaceGit(dir, args...)
	require.NoError(t, err)
	return out
}
