package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCommand_CurrentRepo(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	cmd := registerCmd
	cmd.SetOut(stdout)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Registered")
	assert.Contains(t, output, repo)

	paths, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repo}, paths)
}

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

	// Should not duplicate
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

func TestRegisterCommand_NotARepoCurrentDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	setupGlobalRegistry(t, []string{})

	cmd := registerCmd
	cmd.SetOut(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
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
