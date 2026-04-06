package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCommand_ExplicitPath(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	cmd := registerCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Registered")
	assert.Contains(t, output, repo)

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo}, paths)
}

func TestRegisterCommand_MultipleRepos(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	cmd := registerCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo1, repo2})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, repo1)
	assert.Contains(t, output, repo2)

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo1, repo2}, paths)
}

func TestRegisterCommand_AlreadyRegistered(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	stdout := new(bytes.Buffer)
	cmd := registerCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo})
	require.NoError(t, err)

	// Should not duplicate.
	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo}, paths)
}

func TestRegisterCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	setupGlobalRegistry(t, []string{})

	cmd := registerCmd
	cmd.SetOut(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{dir})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRegisterCommand_PreservesExisting(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1})

	stdout := new(bytes.Buffer)
	cmd := registerCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo2})
	require.NoError(t, err)

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo1, repo2}, paths)
}

// --- discoverRepos tests ---

func TestDiscoverRepos_CurrentDirIsRepo(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{})

	items, err := discoverRepos(repo)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, filepath.Base(repo), items[0].Name)
	assert.Equal(t, repo, items[0].Path)
	assert.False(t, items[0].Registered)
}

func TestDiscoverRepos_ChildRepos(t *testing.T) {
	parent := t.TempDir()
	setupGlobalRegistry(t, []string{})

	exec := &git.RealExecutor{}
	// Create two child repos.
	for _, name := range []string{"repo-a", "repo-b"} {
		child := filepath.Join(parent, name)
		require.NoError(t, os.MkdirAll(child, 0o755))
		_, err := exec.Run(child, "init", "-b", "main")
		require.NoError(t, err)
	}

	// Also create a non-repo directory (should be excluded).
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "not-a-repo"), 0o755))

	items, err := discoverRepos(parent)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "repo-a", items[0].Name)
	assert.Equal(t, "repo-b", items[1].Name)
}

func TestDiscoverRepos_MarksRegistered(t *testing.T) {
	parent := t.TempDir()
	exec := &git.RealExecutor{}

	childA := filepath.Join(parent, "repo-a")
	childB := filepath.Join(parent, "repo-b")
	require.NoError(t, os.MkdirAll(childA, 0o755))
	require.NoError(t, os.MkdirAll(childB, 0o755))
	_, err := exec.Run(childA, "init", "-b", "main")
	require.NoError(t, err)
	_, err = exec.Run(childB, "init", "-b", "main")
	require.NoError(t, err)

	// Pre-register repo-a.
	setupGlobalRegistry(t, []string{childA})

	items, err := discoverRepos(parent)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.True(t, items[0].Registered, "repo-a should be marked as registered")
	assert.False(t, items[1].Registered, "repo-b should not be marked as registered")
}

func TestDiscoverRepos_SkipsDotDirs(t *testing.T) {
	parent := t.TempDir()
	setupGlobalRegistry(t, []string{})

	exec := &git.RealExecutor{}
	// Hidden directory that happens to be a git repo.
	hidden := filepath.Join(parent, ".hidden-repo")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	_, err := exec.Run(hidden, "init", "-b", "main")
	require.NoError(t, err)

	items, err := discoverRepos(parent)
	require.NoError(t, err)
	assert.Empty(t, items, "hidden directories should be skipped")
}

func TestDiscoverRepos_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	setupGlobalRegistry(t, []string{})

	items, err := discoverRepos(dir)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestDiscoverRepos_CurrentDirAndChildren(t *testing.T) {
	// Parent is itself a repo, and has child repos.
	parent := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{})

	exec := &git.RealExecutor{}
	child := filepath.Join(parent, "sub-repo")
	require.NoError(t, os.MkdirAll(child, 0o755))
	_, err := exec.Run(child, "init", "-b", "main")
	require.NoError(t, err)

	items, err := discoverRepos(parent)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, filepath.Base(parent), items[0].Name, "current dir listed first")
	assert.Equal(t, "sub-repo", items[1].Name)
}
