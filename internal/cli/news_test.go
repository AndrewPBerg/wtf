package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/forge"
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

// --- Branch flag tests (switch mode) ---

func TestNewsBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "checkout", "-b", "remote-feature")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "branch", "-D", "remote-feature")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)

	// Re-create the branch since stubFetchExecutor won't actually fetch
	_, err = realExec.Run(dir, "branch", "remote-feature")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runNewBranch(cmd, "remote-feature", wm, exec, nil, true)
	require.NoError(t, err)

	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)
	assert.Contains(t, stderr.String(), "Created worktree at")
}

func TestNewsBranch_InvalidName(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &stubFetchExecutor{real: &git.RealExecutor{}}
	wm := git.NewWorktreeManager(exec)

	stdout := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(stdout)

	err := runNewBranch(cmd, "bad..name", wm, exec, nil, true)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrInvalidBranchName)
}

// --- PR flag tests (switch mode) ---

func TestNewsPR_Integration(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "-b", "pr-1")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)
	cmd := newsCmd

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	tf := &testForge{
		name: "github",
		prs: []forge.PR{
			{
				Number:    1,
				Title:     "Test PR",
				Branch:    "pr-branch",
				Author:    "tester",
				CreatedAt: time.Now(),
				URL:       "https://github.com/test/repo/pull/1",
			},
		},
	}

	ff := func(_ string) (forge.Forge, error) {
		return tf, nil
	}

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	err = runNewPR(cmd, "1", wm, exec, nil, ff, true)
	require.NoError(t, err)

	// In switch mode, path goes to stdout, messages to stderr
	outPath := strings.TrimSpace(stdout.String())
	assert.DirExists(t, outPath)
	assert.Contains(t, stderr.String(), "Checked out")
}

func TestNewsPR_ForgeError(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://bitbucket.org/test/repo.git")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)
	cmd := newsCmd

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	ff := func(_ string) (forge.Forge, error) {
		return nil, fmt.Errorf("unsupported forge")
	}

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	err = runNewPR(cmd, "1", wm, exec, nil, ff, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detecting forge")
}
