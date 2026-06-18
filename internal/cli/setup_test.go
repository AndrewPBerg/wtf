package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDetector(shell string) *setup.ShellDetector {
	return &setup.ShellDetector{
		GetEnv: func(key string) string {
			if key == "SHELL" {
				return "/bin/" + shell
			}
			return ""
		},
	}
}

// --- Shell setup tests (now under "setup shell") ---

func TestSetupShellCommand_Fresh(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Detected shell:")
	assert.Contains(t, output, "bash")
	assert.Contains(t, output, "RC file:")
	assert.Contains(t, output, "Proceed?")
	assert.Contains(t, output, "Added to")

	data, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `eval "$(wtf init bash)"`)
}

func TestSetupShellCommand_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(rcPath, []byte(`eval "$(wtf init zsh)"`+"\n"), 0o644))

	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)

	err := runSetupShell(cmd, newTestDetector("zsh"), rcm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "already configured")
}

func TestSetupShellCommand_Declined(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Aborted")

	// RC file should not exist
	_, err = os.Stat(filepath.Join(dir, ".bashrc"))
	assert.True(t, os.IsNotExist(err))
}

func TestSetupShellCommand_Fish(t *testing.T) {
	dir := t.TempDir()
	fishDir := filepath.Join(dir, ".config", "fish")
	require.NoError(t, os.MkdirAll(fishDir, 0o755))

	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("yes\n"))

	err := runSetupShell(cmd, newTestDetector("fish"), rcm)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(fishDir, "config.fish"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "wtf init fish | source")
}

func TestSetupShellCommand_DetectionFailure(t *testing.T) {
	detector := &setup.ShellDetector{
		GetEnv: func(string) string { return "" },
	}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)

	err := runSetupShell(cmd, detector, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect shell")
}

func TestSetupShellCommand_NilRCManager(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte(`eval "$(wtf init bash)"`+"\n"), 0o644))

	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "already configured")
}

func TestSetupShellCommand_AppendInitError(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("# existing content\n"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(rcPath, 0o644) })

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
}

func TestSetupShellCommand_IsInitPresentError(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("some content"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(rcPath, 0o644) })

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checking rc file")
}

func TestSetupShellCommand_RCFilePathError(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	detector := &setup.ShellDetector{
		GetEnv: func(key string) string {
			if key == "SHELL" {
				return "/bin/tcsh"
			}
			return ""
		},
	}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)

	err := runSetupShell(cmd, detector, rcm)
	assert.Error(t, err)
}

func TestSetupShellCommand_EOFOnInput(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader(""))

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading input")
}

func TestSetupShellCommand_EmptyAnswer(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupShellCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("\n"))

	err := runSetupShell(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

// --- Project setup tests ---

func TestProjectSetup_Default(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runProjectSetup(cmd, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Setup complete")
}

func TestProjectSetup_EnvOnly(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	setupEnvOnly = true
	defer func() { setupEnvOnly = false }()

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runProjectSetup(cmd, runner)
	require.NoError(t, err)
	// Test repo has no .env files in mainDir
	assert.Contains(t, buf.String(), "No env files found")
}

func TestProjectSetup_EnvOnly_CopyEnv(t *testing.T) {
	dir := initCLITestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o600))

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(dir, "setup-copy-env", "main")
	require.NoError(t, err)
	t.Chdir(wtPath)

	setupEnvOnly = true
	setupCopyEnv = true
	defer func() { setupEnvOnly = false; setupCopyEnv = false }()

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err = runProjectSetup(cmd, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), ".env copied")

	target := filepath.Join(wtPath, ".env")
	info, err := os.Lstat(target)
	require.NoError(t, err)
	assert.False(t, info.Mode()&os.ModeSymlink != 0)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "SECRET=1", string(data))
}

func TestProjectSetup_EnvOnly_WithFiles(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Create .env in the repo (acts as main worktree)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644))

	setupEnvOnly = true
	defer func() { setupEnvOnly = false }()

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runProjectSetup(cmd, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), ".env symlinked")
}

func TestProjectSetup_InstallOnly_NoPackageManager(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	setupInstallOnly = true
	defer func() { setupInstallOnly = false }()

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runProjectSetup(cmd, runner)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No package manager detected")
}

func TestProjectSetup_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	runner := setup.NewRunner()

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runProjectSetup(cmd, runner)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestParseMainWorktreePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal", "worktree /code/myrepo\nHEAD abc123\nbranch refs/heads/main\n", "/code/myrepo"},
		{"empty", "", ""},
		{"no worktree prefix", "HEAD abc123\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseMainWorktreePath(tt.input))
		})
	}
}

// mockSetupExecutor is a simple mock for CmdExecutor in CLI tests.
type mockSetupExecutor struct {
	commands []string
}

func (m *mockSetupExecutor) RunShell(_ string, command string) error {
	m.commands = append(m.commands, command)
	return nil
}

func (m *mockSetupExecutor) RunInteractive(_ string, command string) error {
	return m.RunShell("", command)
}
