package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewsCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	newsBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "test-feature", wm, nil)
	require.NoError(t, err)

	// stdout should contain only the path (for cd)
	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)

	// stderr should have the user-facing message
	assert.Contains(t, stderr.String(), "Created worktree at")
	assert.Contains(t, stderr.String(), "test-feature")

	// Verify worktree was actually created
	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 2)
}

func TestNewsCommand_InvalidBranchName(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	newsBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "bad..name", wm, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrInvalidBranchName)
}

func TestNewsCommand_InvalidBase(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	newsBase = "nonexistent-branch"
	defer func() { newsBase = "main" }()

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "new-feature", wm, nil)
	assert.Error(t, err)
}

func TestNewsCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	newsBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "new-feature", wm, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestNewsCommand_WithRunner(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	newsBase = "main"

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "setup-test", wm, runner)
	require.NoError(t, err)

	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)
	assert.Contains(t, stderr.String(), "Created worktree at")
}

func TestNewsCommand_WithConfig(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cfgContent := `
[env]
strategy = "none"

[[setup]]
name = "test"
run = "echo hello"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte(cfgContent), 0o644))

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	newsBase = "main"

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "config-test", wm, runner)
	require.NoError(t, err)

	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)
	assert.Contains(t, stderr.String(), "Created worktree at")
	assert.Contains(t, mock.commands, "echo hello")
}

func TestNewsCommand_SetupFailureIsWarning(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cfgContent := `
[env]
strategy = "bogus"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte(cfgContent), 0o644))

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	newsBase = "main"

	runner := setup.NewRunner()

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "warn-test", wm, runner)
	require.NoError(t, err)

	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)
	assert.Contains(t, stderr.String(), "setup skipped")
}
