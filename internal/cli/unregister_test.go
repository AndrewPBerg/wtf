package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnregisterCommand_CurrentRepo(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{repo})

	stdout := new(bytes.Buffer)
	cmd := unregisterCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Unregistered")
	assert.Contains(t, output, repo)

	// Verify removed from registry
	paths, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestUnregisterCommand_ExplicitPath(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	stdout := new(bytes.Buffer)
	cmd := unregisterCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Unregistered")

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestUnregisterCommand_NotRegistered(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{})

	cmd := unregisterCmd
	cmd.SetOut(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{repo})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestUnregisterCommand_PreservesOtherRepos(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1, repo2})

	stdout := new(bytes.Buffer)
	cmd := unregisterCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{repo1})
	require.NoError(t, err)

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo2}, paths)
}

func TestUnregisterCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	setupGlobalRegistry(t, []string{})

	cmd := unregisterCmd
	cmd.SetOut(new(bytes.Buffer))

	// No args, not in a repo
	err := cmd.RunE(cmd, []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestResolveRepoArg_AbsPath(t *testing.T) {
	result := resolveRepoArg("/home/user/repo")
	assert.Equal(t, "/home/user/repo", result)
}

func TestResolveRepoArg_Dot(t *testing.T) {
	result := resolveRepoArg(".")
	cwd, _ := os.Getwd()
	assert.Equal(t, cwd, result)
}

func TestResolveRepoArg_RegistryMatch(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	name := filepath.Base(repo)
	result := resolveRepoArg(name)
	assert.Equal(t, repo, result)
}

func TestResolveRepoArg_NoMatch(t *testing.T) {
	setupGlobalRegistry(t, []string{})

	// Without match, falls back to abs path
	result := resolveRepoArg("nonexistent-repo")
	assert.True(t, filepath.IsAbs(result))
}
