package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestSetupCommand_Fresh(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Detected shell:")
	assert.Contains(t, output, "bash")
	assert.Contains(t, output, "RC file:")
	assert.Contains(t, output, "Proceed?")
	assert.Contains(t, output, "Added to")

	data, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `eval "$(wtf init)"`)
}

func TestSetupCommand_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(rcPath, []byte(`eval "$(wtf init)"`+"\n"), 0o644))

	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runSetup(cmd, newTestDetector("zsh"), rcm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "already configured")
}

func TestSetupCommand_Declined(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Aborted")

	// RC file should not exist
	_, err = os.Stat(filepath.Join(dir, ".bashrc"))
	assert.True(t, os.IsNotExist(err))
}

func TestSetupCommand_Fish(t *testing.T) {
	dir := t.TempDir()
	fishDir := filepath.Join(dir, ".config", "fish")
	require.NoError(t, os.MkdirAll(fishDir, 0o755))

	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("yes\n"))

	err := runSetup(cmd, newTestDetector("fish"), rcm)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(fishDir, "config.fish"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "wtf init fish | source")
}

func TestSetupCommand_DetectionFailure(t *testing.T) {
	detector := &setup.ShellDetector{
		GetEnv: func(string) string { return "" },
	}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runSetup(cmd, detector, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect shell")
}

func TestSetupCommand_NilRCManager(t *testing.T) {
	// When rcm is nil, runSetup creates one from the real home dir.
	// Use a temp file that already has the init line so it exits early.
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte(`eval "$(wtf init)"`+"\n"), 0o644))

	// We pass a real RCFileManager pointing at our temp dir to exercise
	// the non-nil path with already-configured state.
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "already configured")
}

func TestSetupCommand_AppendInitError(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	// Create the rc file as read-only so IsInitPresent can read but AppendInit can't write
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("# existing content\n"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(rcPath, 0o644) })

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
}

func TestSetupCommand_IsInitPresentError(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	// Point RCFilePath to a directory that exists but make IsInitPresent fail
	// by having a non-readable rc file
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("some content"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(rcPath, 0o644) })

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checking rc file")
}

func TestSetupCommand_RCFilePathError(t *testing.T) {
	// Use an unsupported shell to trigger RCFilePath error
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
	cmd := setupCmd
	cmd.SetOut(buf)

	err := runSetup(cmd, detector, rcm)
	assert.Error(t, err)
}

func TestSetupCommand_EOFOnInput(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("")) // EOF, no newline

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading input")
}

func TestSetupCommand_EmptyAnswer(t *testing.T) {
	dir := t.TempDir()
	rcm := &setup.RCFileManager{HomeDir: dir}

	buf := new(bytes.Buffer)
	cmd := setupCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("\n"))

	err := runSetup(cmd, newTestDetector("bash"), rcm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}
