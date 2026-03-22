package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRepos_Empty(t *testing.T) {
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	cmd := reposCmd
	cmd.SetOut(stdout)

	err := runRepos(cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "No registered repos")
}

func TestRunRepos_WithRepos(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	stdout := new(bytes.Buffer)
	cmd := reposCmd
	cmd.SetOut(stdout)

	err := runRepos(cmd)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, filepath.Base(repo))
	assert.Contains(t, output, "1 repo(s) registered")
}

func TestRunRepos_JSON(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	jsonOutput = true
	defer func() { jsonOutput = false }()

	stdout := new(bytes.Buffer)
	cmd := reposCmd
	cmd.SetOut(stdout)

	err := runRepos(cmd)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), repo)
}

func TestRunRepos_HighlightsCurrentRepo(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})
	t.Chdir(repo)

	stdout := new(bytes.Buffer)
	cmd := reposCmd
	cmd.SetOut(stdout)

	err := runRepos(cmd)
	require.NoError(t, err)
	// The output should contain the repo name (highlighted)
	assert.Contains(t, stdout.String(), filepath.Base(repo))
}

func TestRunRepos_LoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("bad"), 0o644))

	stdout := new(bytes.Buffer)
	cmd := reposCmd
	cmd.SetOut(stdout)

	err := runRepos(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading registry")
}
