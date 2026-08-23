package setup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCompletionFile_OverwritesExistingFile(t *testing.T) {
	home := t.TempDir()

	path, err := WriteCompletionFile(Zsh, home, "first")
	require.NoError(t, err)
	pathAgain, err := WriteCompletionFile(Zsh, home, "second")
	require.NoError(t, err)
	assert.Equal(t, path, pathAgain)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "second", string(data))
}

func TestWriteCompletionFile_DirectoryWriteError(t *testing.T) {
	home := t.TempDir()
	path := CompletionFilePath(Fish, home)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.Mkdir(path, 0o755))

	_, err := WriteCompletionFile(Fish, home, "content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing completion file")
}

func TestDiscoverEnvFiles_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".git", "node_modules", ".venv", "vendor", "__pycache__", ".next", ".nuxt", "dist", "build"} {
		path := filepath.Join(dir, name, "nested")
		require.NoError(t, os.MkdirAll(path, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(path, ".env.secret"), []byte("x"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".environment"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.env"), []byte("x"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "link-target"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "link-target", ".env"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir, "link-target"), filepath.Join(dir, "link")))

	files, err := DiscoverEnvFiles(dir)
	require.NoError(t, err)
	assert.Contains(t, files, ".environment")
	assert.NotContains(t, files, "plain.env")
	assert.NotContains(t, files, filepath.Join("link", ".env"))
	for _, name := range []string{".git", "node_modules", ".venv", "vendor", "__pycache__", ".next", ".nuxt", "dist", "build"} {
		assert.NotContains(t, files, filepath.Join(name, "nested", ".env.secret"))
	}

	missing, err := DiscoverEnvFiles(filepath.Join(dir, "does-not-exist"))
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestRCFileManager_FishPathWithCustomHome(t *testing.T) {
	m := &RCFileManager{HomeDir: filepath.Join("tmp", "home")}
	path, err := m.RCFilePath(Fish)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "home", ".config", "fish", "config.fish"), path)
}

func TestSymlinkDirs_SkipsNonDirectoriesAndExistingDestinations(t *testing.T) {
	mainDir, targetDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file"), []byte("x"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(mainDir, "dir"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(mainDir, "existing"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "existing"), []byte("keep"), 0o644))

	linked, err := symlinkDirs(mainDir, targetDir, []string{"missing", "file", "dir", "existing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dir"}, linked)
	got, err := os.Readlink(filepath.Join(targetDir, "dir"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("..", filepath.Base(mainDir), "dir"), got)
}

func TestRunSetup_SymlinkDirectoryError(t *testing.T) {
	mainDir, parent := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(mainDir, ".venv"), 0o755))
	target := filepath.Join(parent, "target-file")
	require.NoError(t, os.WriteFile(target, []byte("not a directory"), 0o644))

	runner := &Runner{CmdExec: newMockCmdExecutor(), EnvHandler: NewEnvFileHandler(), Out: &bytes.Buffer{}}
	err := runner.RunSetup(mainDir, target, Options{SkipInstall: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlinking directories")
}

func TestNewRunner_UsesConcreteDefaults(t *testing.T) {
	runner := NewRunner()
	require.NotNil(t, runner)
	assert.IsType(t, &RealCmdExecutor{}, runner.CmdExec)
	assert.IsType(t, &EnvFileHandler{}, runner.EnvHandler)
}

func TestRealCmdExecutor_RunShell(t *testing.T) {
	dir := t.TempDir()
	executor := &RealCmdExecutor{}
	require.NoError(t, executor.RunShell(dir, "true"))
	assert.Error(t, executor.RunShell(dir, "false"))
}

func TestRealCmdExecutor_RunInteractive(t *testing.T) {
	executor := &RealCmdExecutor{}
	require.NoError(t, executor.RunInteractive(t.TempDir(), "true"))
	assert.Error(t, executor.RunInteractive(t.TempDir(), "false"))
}

func TestShellDetector_SafeFallbackErrors(t *testing.T) {
	tests := []struct {
		name     string
		detector *ShellDetector
		contains string
	}{
		{"nil parent reader", &ShellDetector{GetEnv: func(string) string { return "" }}, "could not detect shell"},
		{"unsupported environment shell", &ShellDetector{
			GetEnv:         func(string) string { return "/bin/tcsh" },
			ReadParentComm: func(int) (string, error) { return "bash", nil },
		}, "unsupported shell"},
		{"unsupported parent shell", &ShellDetector{
			GetEnv:         func(string) string { return "" },
			ReadParentComm: func(int) (string, error) { return "tcsh", nil },
		}, "could not detect shell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.detector.Detect("")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

func TestCopyFile_SourceDirectoryError(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(dir, filepath.Join(dir, "dst"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copying data")
}

func TestRunSetup_UnknownEnvStrategy(t *testing.T) {
	mainDir, targetDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	runner := &Runner{CmdExec: newMockCmdExecutor(), EnvHandler: NewEnvFileHandler(), Out: &bytes.Buffer{}}
	err := runner.RunSetup(mainDir, targetDir, Options{EnvStrategy: "move", SkipInstall: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handling env files")
	assert.Contains(t, err.Error(), fmt.Sprintf("unknown env strategy: %q", "move"))
}
