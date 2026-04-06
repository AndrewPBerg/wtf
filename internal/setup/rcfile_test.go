package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRCFileManager(t *testing.T) {
	m, err := NewRCFileManager()
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.NotEmpty(t, m.HomeDir)
}

func TestRCFilePath(t *testing.T) {
	m := &RCFileManager{HomeDir: "/home/user"}

	tests := []struct {
		shell Shell
		want  string
	}{
		{Bash, "/home/user/.bashrc"},
		{Zsh, "/home/user/.zshrc"},
		{Fish, "/home/user/.config/fish/config.fish"},
	}

	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			got, err := m.RCFilePath(tt.shell)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRCFilePath_Unsupported(t *testing.T) {
	m := &RCFileManager{HomeDir: "/home/user"}
	_, err := m.RCFilePath("ksh")
	assert.Error(t, err)
}

func TestInitLine(t *testing.T) {
	tests := []struct {
		shell Shell
		want  string
	}{
		{Bash, `eval "$(wtf init bash)"`},
		{Zsh, `eval "$(wtf init zsh)"`},
		{Fish, "wtf init fish | source"},
	}

	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			assert.Equal(t, tt.want, InitLine(tt.shell))
		})
	}
}

func TestIsInitPresent(t *testing.T) {
	dir := t.TempDir()

	t.Run("file does not exist", func(t *testing.T) {
		present, err := IsInitPresent(filepath.Join(dir, "nonexistent"))
		require.NoError(t, err)
		assert.False(t, present)
	})

	t.Run("file exists without init", func(t *testing.T) {
		path := filepath.Join(dir, ".bashrc")
		require.NoError(t, os.WriteFile(path, []byte("export PATH=/usr/bin\n"), 0o644))
		present, err := IsInitPresent(path)
		require.NoError(t, err)
		assert.False(t, present)
	})

	t.Run("file exists with init", func(t *testing.T) {
		path := filepath.Join(dir, ".zshrc")
		require.NoError(t, os.WriteFile(path, []byte("eval \"$(wtf init)\"\n"), 0o644))
		present, err := IsInitPresent(path)
		require.NoError(t, err)
		assert.True(t, present)
	})

	t.Run("file exists with fish init", func(t *testing.T) {
		path := filepath.Join(dir, "config.fish")
		require.NoError(t, os.WriteFile(path, []byte("wtf init fish | source\n"), 0o644))
		present, err := IsInitPresent(path)
		require.NoError(t, err)
		assert.True(t, present)
	})
}

func TestAppendInit(t *testing.T) {
	t.Run("creates new file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".bashrc")
		require.NoError(t, AppendInit(path, Bash))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "WorkTreeForge shell integration")
		assert.Contains(t, string(data), `eval "$(wtf init bash)"`)
	})

	t.Run("appends to existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".zshrc")
		require.NoError(t, os.WriteFile(path, []byte("# existing content\n"), 0o644))
		require.NoError(t, AppendInit(path, Zsh))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "# existing content")
		assert.Contains(t, string(data), `eval "$(wtf init zsh)"`)
	})

	t.Run("fish shell", func(t *testing.T) {
		dir := t.TempDir()
		fishDir := filepath.Join(dir, ".config", "fish")
		require.NoError(t, os.MkdirAll(fishDir, 0o755))
		path := filepath.Join(fishDir, "config.fish")
		require.NoError(t, AppendInit(path, Fish))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "wtf init fish | source")
	})

	t.Run("unwritable path", func(t *testing.T) {
		err := AppendInit("/nonexistent/dir/.bashrc", Bash)
		assert.Error(t, err)
	})
}

func TestIsInitPresent_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := IsInitPresent(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading rc file")
}
