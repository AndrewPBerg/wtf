package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
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
}

func TestRunSetup_NilConfig_WithPackageManager(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(""), 0o644))

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(nil, "", dir, "main")
	require.NoError(t, err)

	assert.Equal(t, []string{"pnpm install"}, mock.commandStrings())
}

func TestRunSetup_NilConfig_NoPackageManager(t *testing.T) {
	dir := t.TempDir()

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(nil, "", dir, "main")
	require.NoError(t, err)

	assert.Empty(t, mock.commands)
}

func TestRunSetup_WithConfig_FullFlow(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Create env file and lockfile in main
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("S=1"), 0o644))
	// Create lockfile in target (where install runs)
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "package-lock.json"), []byte(""), 0o644))

	cfg := &config.ProjectConfig{
		Env: config.EnvConfig{
			Strategy: "copy",
			Files:    []string{".env"},
		},
		Setup: []config.SetupStep{
			{Name: "compile", Run: "make build"},
		},
		Hooks: config.HooksConfig{
			OnCreate: []string{"echo done"},
		},
	}

	mock := newMockCmdExecutor()
	envHandler := &EnvFileHandler{
		CopyFile: copyFile,
	}
	runner := &Runner{CmdExec: mock, EnvHandler: envHandler}

	err := runner.RunSetup(cfg, mainDir, targetDir, "feature/test")
	require.NoError(t, err)

	cmds := mock.commandStrings()
	// Should be: npm install, make build, echo done
	assert.Equal(t, []string{"npm install", "make build", "echo done"}, cmds)

	// Verify env file was copied
	data, err := os.ReadFile(filepath.Join(targetDir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "S=1", string(data))
}

func TestRunSetup_ConditionSkipping(t *testing.T) {
	targetDir := t.TempDir()

	cfg := &config.ProjectConfig{
		Setup: []config.SetupStep{
			{Name: "always", Run: "echo always"},
			{Name: "skip", Run: "echo skip", If: "branch contains 'hotfix'"},
			{Name: "run", Run: "echo run", If: "branch contains 'feature'"},
		},
	}

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(cfg, "", targetDir, "feature/test")
	require.NoError(t, err)

	cmds := mock.commandStrings()
	assert.Equal(t, []string{"echo always", "echo run"}, cmds)
}

func TestRunSetup_ConditionError(t *testing.T) {
	targetDir := t.TempDir()

	cfg := &config.ProjectConfig{
		Setup: []config.SetupStep{
			{Name: "bad", Run: "echo bad", If: "invalid condition"},
		},
	}

	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(cfg, "", targetDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "evaluating condition")
}

func TestRunSetup_StepFailure(t *testing.T) {
	targetDir := t.TempDir()

	cfg := &config.ProjectConfig{
		Setup: []config.SetupStep{
			{Name: "fail", Run: "bad command"},
		},
	}

	mock := newMockCmdExecutor()
	mock.failOn["bad command"] = fmt.Errorf("command failed")
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(cfg, "", targetDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "running setup step")
}

func TestRunSetup_EnvError(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	cfg := &config.ProjectConfig{
		Env: config.EnvConfig{
			Strategy: "symlink",
			Files:    []string{".env"},
		},
	}

	h := &EnvFileHandler{
		Symlink: func(_, _ string) error { return fmt.Errorf("symlink broken") },
	}
	mock := newMockCmdExecutor()
	runner := &Runner{CmdExec: mock, EnvHandler: h}

	err := runner.RunSetup(cfg, mainDir, targetDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handling env files")
}

func TestRunSetup_HookFailure(t *testing.T) {
	targetDir := t.TempDir()

	cfg := &config.ProjectConfig{
		Hooks: config.HooksConfig{
			OnCreate: []string{"failing hook"},
		},
	}

	mock := newMockCmdExecutor()
	mock.failOn["failing hook"] = fmt.Errorf("hook failed")
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(cfg, "", targetDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "running on_create hooks")
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

func TestRunSetup_InstallFailure(t *testing.T) {
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "yarn.lock"), []byte(""), 0o644))

	cfg := &config.ProjectConfig{}

	mock := newMockCmdExecutor()
	mock.failOn["yarn install"] = fmt.Errorf("yarn not found")
	runner := &Runner{CmdExec: mock, EnvHandler: NewEnvFileHandler()}

	err := runner.RunSetup(cfg, "", targetDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auto setup")
}

func TestRunSetup_Ordering(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "pnpm-lock.yaml"), []byte(""), 0o644))

	var order []string

	cfg := &config.ProjectConfig{
		Env: config.EnvConfig{
			Strategy: "copy",
			Files:    []string{".env"},
		},
		Setup: []config.SetupStep{
			{Name: "step1", Run: "step1 cmd"},
		},
		Hooks: config.HooksConfig{
			OnCreate: []string{"hook cmd"},
		},
	}

	mock := newMockCmdExecutor()
	envHandler := &EnvFileHandler{
		CopyFile: func(_, _ string) error {
			order = append(order, "env")
			return nil
		},
	}

	origRunShell := mock.RunShell
	_ = origRunShell
	// Wrap to track ordering
	wrappedMock := &orderTrackingExecutor{inner: mock, order: &order}

	runner := &Runner{CmdExec: wrappedMock, EnvHandler: envHandler}

	err := runner.RunSetup(cfg, mainDir, targetDir, "main")
	require.NoError(t, err)

	assert.Equal(t, []string{"env", "pnpm install", "step1 cmd", "hook cmd"}, order)
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
