package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleEnvFiles_Symlink(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Create env files in main
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("SECRET=1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env.local"), []byte("LOCAL=1"), 0o644))

	var symlinks []struct{ old, new string }
	h := &EnvFileHandler{
		Symlink: func(old, new string) error {
			symlinks = append(symlinks, struct{ old, new string }{old, new})
			return nil
		},
	}

	err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env", ".env.local", ".env.missing"})
	require.NoError(t, err)

	// Only existing files should be symlinked
	assert.Len(t, symlinks, 2)
	assert.Equal(t, filepath.Join(targetDir, ".env"), symlinks[0].new)
	assert.Equal(t, filepath.Join(targetDir, ".env.local"), symlinks[1].new)
}

func TestHandleEnvFiles_Copy(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("KEY=val"), 0o644))

	h := NewEnvFileHandler()

	err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetDir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "KEY=val", string(data))
}

func TestHandleEnvFiles_None(t *testing.T) {
	h := NewEnvFileHandler()
	err := h.HandleEnvFiles("/main", "/target", "none", []string{".env"})
	assert.NoError(t, err)
}

func TestHandleEnvFiles_EmptyStrategy(t *testing.T) {
	h := NewEnvFileHandler()
	err := h.HandleEnvFiles("/main", "/target", "", []string{".env"})
	assert.NoError(t, err)
}

func TestHandleEnvFiles_DefaultFiles(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Create all default env files
	for _, f := range DefaultEnvFiles {
		require.NoError(t, os.WriteFile(filepath.Join(mainDir, f), []byte("x"), 0o644))
	}

	var copied []string
	h := &EnvFileHandler{
		CopyFile: func(_, dst string) error {
			copied = append(copied, filepath.Base(dst))
			return nil
		},
	}

	err := h.HandleEnvFiles(mainDir, targetDir, "copy", nil)
	require.NoError(t, err)

	assert.Equal(t, DefaultEnvFiles, copied)
}

func TestHandleEnvFiles_UnknownStrategy(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	h := NewEnvFileHandler()
	err := h.HandleEnvFiles(mainDir, targetDir, "move", []string{".env"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown env strategy")
}

func TestHandleEnvFiles_SymlinkError(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	h := &EnvFileHandler{
		Symlink: func(_, _ string) error {
			return os.ErrPermission
		},
	}

	err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "symlinking")
}

func TestHandleEnvFiles_CopyError(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	h := &EnvFileHandler{
		CopyFile: func(_, _ string) error {
			return os.ErrPermission
		},
	}

	err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "copying")
}

func TestHandleEnvFiles_SkipsMissingFiles(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	var copied []string
	h := &EnvFileHandler{
		CopyFile: func(_, dst string) error {
			copied = append(copied, filepath.Base(dst))
			return nil
		},
	}

	// No files exist in mainDir
	err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env", ".env.local"})
	require.NoError(t, err)
	assert.Empty(t, copied)
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	require.NoError(t, os.WriteFile(src, []byte("hello world"), 0o644))

	err := copyFile(src, dst)
	require.NoError(t, err)

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "opening source")
}

func TestHandleEnvFiles_RealSymlink(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("REAL=yes"), 0o644))

	h := NewEnvFileHandler()
	err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	require.NoError(t, err)

	// Verify symlink was created and resolves correctly
	link := filepath.Join(targetDir, ".env")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.NotEmpty(t, target)

	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "REAL=yes", string(data))
}
