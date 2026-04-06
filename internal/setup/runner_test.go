package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCmdExecutor records commands and optionally returns errors.
type mockCmdExecutor struct {
	commands []struct {
		dir     string
		command string
	}
	failOn map[string]error
}

func newMockCmdExecutor() *mockCmdExecutor {
	return &mockCmdExecutor{failOn: make(map[string]error)}
}

func (m *mockCmdExecutor) RunShell(dir, command string) error {
	m.commands = append(m.commands, struct {
		dir     string
		command string
	}{dir, command})
	if err, ok := m.failOn[command]; ok {
		return err
	}
	return nil
}

func (m *mockCmdExecutor) RunInteractive(dir, command string) error {
	return m.RunShell(dir, command)
}

func (m *mockCmdExecutor) commandStrings() []string {
	var cmds []string
	for _, c := range m.commands {
		cmds = append(cmds, c.command)
	}
	return cmds
}

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	require.NotNil(t, r)
	require.NotNil(t, r.CmdExec)
	require.NotNil(t, r.EnvHandler)
	require.NotNil(t, r.Out)
}

func TestRunSetup_Default_WithPackageManager(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "pnpm-lock.yaml"), []byte(""), 0o644))

	mock := newMockCmdExecutor()
	var buf bytes.Buffer
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: &buf}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	assert.Equal(t, []string{"pnpm install"}, mock.commandStrings())
	assert.Contains(t, buf.String(), "pnpm install")
}

func TestRunSetup_Default_NoPackageManager(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	assert.Empty(t, mock.commands)
}

func TestRunSetup_SymlinksEnvFiles(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("SECRET=1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env.local"), []byte("LOCAL=1"), 0o644))

	var symlinked []string
	envHandler := &EnvFileHandler{
		Symlink: func(oldname, newname string) error {
			symlinked = append(symlinked, filepath.Base(newname))
			return os.Symlink(oldname, newname)
		},
		CopyFile: copyFile,
	}

	var buf bytes.Buffer
	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: envHandler, Out: &buf}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	assert.Contains(t, symlinked, ".env")
	assert.Contains(t, symlinked, ".env.local")
	assert.Contains(t, buf.String(), ".env")
	assert.Contains(t, buf.String(), ".env.local")
}

func TestRunSetup_CopyStrategy(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("S=1"), 0o644))

	mock := newMockCmdExecutor()
	envHandler := &EnvFileHandler{
		CopyFile: copyFile,
	}
	runner := &Runner{CmdExec: mock, EnvHandler: envHandler, Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{EnvStrategy: "copy"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetDir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "S=1", string(data))
}

func TestRunSetup_SkipEnv(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	mock := newMockCmdExecutor()
	envHandler := &EnvFileHandler{
		Symlink: func(_, _ string) error {
			t.Fatal("should not symlink when SkipEnv is set")
			return nil
		},
	}
	runner := &Runner{CmdExec: mock, EnvHandler: envHandler, Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{SkipEnv: true})
	require.NoError(t, err)
}

func TestRunSetup_SkipInstall(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "pnpm-lock.yaml"), []byte(""), 0o644))

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{SkipInstall: true})
	require.NoError(t, err)

	assert.Empty(t, mock.commands)
}

func TestRunSetup_EnvError(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	h := &EnvFileHandler{
		Symlink: func(_, _ string) error { return fmt.Errorf("symlink broken") },
	}
	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: h, Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handling env files")
}

func TestRunSetup_InstallFailure(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "yarn.lock"), []byte(""), 0o644))

	mock := newMockCmdExecutor()
	mock.failOn["yarn install"] = fmt.Errorf("yarn not found")
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auto setup")
}

func TestRunSetup_Ordering(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "pnpm-lock.yaml"), []byte(""), 0o644))

	var order []string

	mock := newMockCmdExecutor()
	envHandler := &EnvFileHandler{
		CopyFile: func(_, _ string) error {
			order = append(order, "env")
			return nil
		},
	}

	wrappedMock := &orderTrackingExecutor{inner: mock, order: &order}
	runner := &Runner{CmdExec: wrappedMock, EnvHandler: envHandler, Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{EnvStrategy: "copy"})
	require.NoError(t, err)

	assert.Equal(t, []string{"env", "pnpm install"}, order)
}

func TestRunHooks(t *testing.T) {
	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock}

	err := runner.RunHooks([]string{"echo a", "echo b", "", "  "}, "/dir")
	require.NoError(t, err)

	assert.Equal(t, []string{"echo a", "echo b"}, mock.commandStrings())
}

func TestRunHooks_Empty(t *testing.T) {
	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock}

	err := runner.RunHooks(nil, "/dir")
	require.NoError(t, err)
	assert.Empty(t, mock.commands)
}

func TestRunHooks_Failure(t *testing.T) {
	mock := newMockCmdExecutor()
	mock.failOn["fail cmd"] = fmt.Errorf("boom")
	runner := &Runner{CmdExec: mock}

	err := runner.RunHooks([]string{"ok cmd", "fail cmd", "never"}, "/dir")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "running hook")

	// "never" should not have been executed
	assert.Len(t, mock.commands, 2)
}

type orderTrackingExecutor struct {
	inner *mockCmdExecutor
	order *[]string
}

func (o *orderTrackingExecutor) RunShell(dir, command string) error {
	*o.order = append(*o.order, command)
	return o.inner.RunShell(dir, command)
}

func (o *orderTrackingExecutor) RunInteractive(dir, command string) error {
	*o.order = append(*o.order, command)
	return o.inner.RunInteractive(dir, command)
}

func TestRunSetup_SymlinksVenv(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Create .venv directory in main worktree
	require.NoError(t, os.Mkdir(filepath.Join(mainDir, ".venv"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".venv", "pyvenv.cfg"), []byte("home = /usr/bin"), 0o644))

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	link := filepath.Join(targetDir, ".venv")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0, ".venv should be a symlink")
}

func TestRunSetup_SkipsVenvWhenMissing(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	// No .venv in mainDir

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	_, err = os.Lstat(filepath.Join(targetDir, ".venv"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunSetup_SkipsVenvWhenAlreadyExists(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(mainDir, ".venv"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(targetDir, ".venv"), 0o755))

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	// Should still be a real directory, not a symlink
	info, err := os.Lstat(filepath.Join(targetDir, ".venv"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.False(t, info.Mode()&os.ModeSymlink != 0)
}

func TestRunSetup_OutputsEnvAndInstall(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "app", ".env.local"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "uv.lock"), []byte(""), 0o644))

	mock := newMockCmdExecutor()
	var buf bytes.Buffer
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: &buf}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, ".env")
	assert.Contains(t, output, filepath.Join("app", ".env.local"))
	assert.Contains(t, output, "uv sync")
}

func TestRunSetup_DiscoversSubdirEnvFiles(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Root env file
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("ROOT=1"), 0o644))
	// Subdir env file
	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "app", ".env"), []byte("APP=1"), 0o644))
	// Target app dir exists (as in a real worktree checkout)
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "app"), 0o755))

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler(), Out: io.Discard}

	err := runner.RunSetup(mainDir, targetDir, Options{})
	require.NoError(t, err)

	// Both root and subdir env files should be symlinked
	rootLink := filepath.Join(targetDir, ".env")
	data, err := os.ReadFile(rootLink)
	require.NoError(t, err)
	assert.Equal(t, "ROOT=1", string(data))

	appLink := filepath.Join(targetDir, "app", ".env")
	data, err = os.ReadFile(appLink)
	require.NoError(t, err)
	assert.Equal(t, "APP=1", string(data))
}
