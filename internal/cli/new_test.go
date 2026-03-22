package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "test-feature", wm, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Created worktree at")
	assert.Contains(t, output, "test-feature")

	// Verify worktree was actually created
	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 2)
}

func TestNewCommand_InvalidBase(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "nonexistent-branch"
	defer func() { newBase = "main" }()

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "new-feature", wm, nil)
	assert.Error(t, err)
}

func TestNewCommand_InvalidBranchName(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "bad..name", wm, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrInvalidBranchName)
}

func TestNewCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "new-feature", wm, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestNewCommand_WithRunner(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)
	newBase = "main"

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "setup-test", wm, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
}

func TestNewCommand_WithConfig(t *testing.T) {
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

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)
	newBase = "main"

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "config-test", wm, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
	assert.Contains(t, mock.commands, "echo hello")
}

func TestNewCommand_SetupFailureIsWarning(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cfgContent := `
[env]
strategy = "bogus"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte(cfgContent), 0o644))

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)
	newBase = "main"

	runner := setup.NewRunner()

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "warn-test", wm, runner)
	// Should succeed — setup failures are warnings
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
	assert.Contains(t, stderr.String(), "setup skipped")
}
