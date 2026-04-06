package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsCompletionConfigured_InlineViaInit(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte(`eval "$(wtf init)"`), 0o644))

	status, err := IsCompletionConfigured(Bash, rcPath, dir)
	require.NoError(t, err)
	assert.True(t, status.Configured)
	assert.Equal(t, "inline", status.Method)
}

func TestIsCompletionConfigured_FileExists(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("# nothing"), 0o644))

	// Create the completion file
	compPath := CompletionFilePath(Bash, dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(compPath), 0o755))
	require.NoError(t, os.WriteFile(compPath, []byte("# completions"), 0o644))

	status, err := IsCompletionConfigured(Bash, rcPath, dir)
	require.NoError(t, err)
	assert.True(t, status.Configured)
	assert.Equal(t, "file", status.Method)
	assert.Equal(t, compPath, status.Path)
}

func TestIsCompletionConfigured_NotConfigured(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("# nothing"), 0o644))

	status, err := IsCompletionConfigured(Bash, rcPath, dir)
	require.NoError(t, err)
	assert.False(t, status.Configured)
}

func TestIsCompletionConfigured_NoRCFile(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc") // does not exist

	status, err := IsCompletionConfigured(Bash, rcPath, dir)
	require.NoError(t, err)
	assert.False(t, status.Configured)
}

func TestCompletionFilePath(t *testing.T) {
	tests := []struct {
		shell Shell
		want  string
	}{
		{Bash, ".local/share/bash-completion/completions/wtf"},
		{Zsh, ".zsh/completions/_wtf"},
		{Fish, ".config/fish/completions/wtf.fish"},
		{"unsupported", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			got := CompletionFilePath(tt.shell, "/home/test")
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, filepath.Join("/home/test", tt.want), got)
			}
		})
	}
}

func TestWriteCompletionFile(t *testing.T) {
	dir := t.TempDir()
	content := "# test completions\n"

	path, err := WriteCompletionFile(Bash, dir, content)
	require.NoError(t, err)
	assert.Equal(t, CompletionFilePath(Bash, dir), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestWriteCompletionFile_UnsupportedShell(t *testing.T) {
	_, err := WriteCompletionFile("unsupported", t.TempDir(), "content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}

func TestWriteCompletionFile_DirCreationError(t *testing.T) {
	// Use an unwritable parent so MkdirAll fails
	dir := t.TempDir()
	unwritable := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(unwritable, 0o555))
	t.Cleanup(func() { _ = os.Chmod(unwritable, 0o755) })

	_, err := WriteCompletionFile(Bash, filepath.Join(unwritable, "sub"), "content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating completion directory")
}

func TestIsCompletionConfigured_RCReadError(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("content"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(rcPath, 0o644) })

	_, err := IsCompletionConfigured(Bash, rcPath, dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checking completion status")
}

func TestIsCompletionConfigured_AllShells(t *testing.T) {
	for _, shell := range []Shell{Bash, Zsh, Fish} {
		t.Run(string(shell), func(t *testing.T) {
			dir := t.TempDir()
			rcPath := filepath.Join(dir, "rc")
			require.NoError(t, os.WriteFile(rcPath, []byte("wtf init"), 0o644))

			status, err := IsCompletionConfigured(shell, rcPath, dir)
			require.NoError(t, err)
			assert.True(t, status.Configured)
			assert.Equal(t, "inline", status.Method)
		})
	}
}
