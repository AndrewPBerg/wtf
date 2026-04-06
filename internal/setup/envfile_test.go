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

	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env", ".env.local", ".env.missing"})
	require.NoError(t, err)

	// Only existing files should be symlinked
	assert.Len(t, symlinks, 2)
	assert.Equal(t, filepath.Join(targetDir, ".env"), symlinks[0].new)
	assert.Equal(t, filepath.Join(targetDir, ".env.local"), symlinks[1].new)
	assert.Equal(t, []string{".env", ".env.local"}, handled)
}

func TestHandleEnvFiles_Copy(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("KEY=val"), 0o644))

	h := NewEnvFileHandler()

	handled, err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetDir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "KEY=val", string(data))
	assert.Equal(t, []string{".env"}, handled)
}

func TestHandleEnvFiles_None(t *testing.T) {
	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles("/main", "/target", "none", []string{".env"})
	assert.NoError(t, err)
	assert.Empty(t, handled)
}

func TestHandleEnvFiles_EmptyStrategy(t *testing.T) {
	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles("/main", "/target", "", []string{".env"})
	assert.NoError(t, err)
	assert.Empty(t, handled)
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

	handled, err := h.HandleEnvFiles(mainDir, targetDir, "copy", nil)
	require.NoError(t, err)

	assert.Equal(t, DefaultEnvFiles, copied)
	assert.Equal(t, DefaultEnvFiles, handled)
}

func TestHandleEnvFiles_UnknownStrategy(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("x"), 0o644))

	h := NewEnvFileHandler()
	_, err := h.HandleEnvFiles(mainDir, targetDir, "move", []string{".env"})
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

	_, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
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

	_, err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env"})
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
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "copy", []string{".env", ".env.local"})
	require.NoError(t, err)
	assert.Empty(t, copied)
	assert.Empty(t, handled)
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

func TestCopyFile_DestinationError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

	// Destination in a non-existent directory
	err := copyFile(src, filepath.Join(dir, "nonexistent", "dst.txt"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating destination")
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o755))

	err := copyFile(src, dst)
	require.NoError(t, err)

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestHandleEnvFiles_RealSymlink(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("REAL=yes"), 0o644))

	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	require.NoError(t, err)
	assert.Equal(t, []string{".env"}, handled)

	// Verify symlink was created and resolves correctly
	link := filepath.Join(targetDir, ".env")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.NotEmpty(t, target)

	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "REAL=yes", string(data))
}

func TestHandleEnvFiles_OverwritesExistingFile(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("FROM_MAIN=1"), 0o644))
	// Pre-existing file at target (e.g. checked out from git)
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, ".env"), []byte("OLD=1"), 0o644))

	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	require.NoError(t, err)
	assert.Equal(t, []string{".env"}, handled)

	link := filepath.Join(targetDir, ".env")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0, ".env should be a symlink")

	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "FROM_MAIN=1", string(data))
}

func TestHandleEnvFiles_SkipsCorrectSymlink(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, ".env"), []byte("VAL=1"), 0o644))

	h := NewEnvFileHandler()
	// Create the symlink first
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	require.NoError(t, err)
	assert.Equal(t, []string{".env"}, handled)

	// Run again — should skip without error and return empty handled
	callCount := 0
	h2 := &EnvFileHandler{
		Symlink: func(old, new string) error {
			callCount++
			return os.Symlink(old, new)
		},
	}
	handled, err = h2.HandleEnvFiles(mainDir, targetDir, "symlink", []string{".env"})
	require.NoError(t, err)
	assert.Equal(t, 0, callCount, "should not re-symlink when already correct")
	assert.Empty(t, handled)
}

func TestHandleEnvFiles_SubdirectoryFiles(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	// Create env file in a subdirectory
	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "app", ".env"), []byte("APP=1"), 0o644))
	// Target subdirectory exists (as it would in a worktree checkout)
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "app"), 0o755))

	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{filepath.Join("app", ".env")})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("app", ".env")}, handled)

	link := filepath.Join(targetDir, "app", ".env")
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "APP=1", string(data))
}

func TestHandleEnvFiles_CreatesParentDir(t *testing.T) {
	mainDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "packages", "api"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "packages", "api", ".env"), []byte("X=1"), 0o644))

	h := NewEnvFileHandler()
	handled, err := h.HandleEnvFiles(mainDir, targetDir, "symlink", []string{filepath.Join("packages", "api", ".env")})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("packages", "api", ".env")}, handled)

	link := filepath.Join(targetDir, "packages", "api", ".env")
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, "X=1", string(data))
}

func TestDiscoverEnvFiles_RootAndSubdir(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.local"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", ".env"), []byte("x"), 0o644))
	// Non-env file should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "main.go"), []byte("x"), 0o644))

	files, err := DiscoverEnvFiles(dir)
	require.NoError(t, err)

	assert.Contains(t, files, ".env")
	assert.Contains(t, files, ".env.local")
	assert.Contains(t, files, filepath.Join("app", ".env"))
	assert.NotContains(t, files, filepath.Join("app", "main.go"))
}

func TestDiscoverEnvFiles_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "pkg", ".env"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644))

	files, err := DiscoverEnvFiles(dir)
	require.NoError(t, err)

	assert.Equal(t, []string{".env"}, files)
}

func TestDiscoverEnvFiles_Empty(t *testing.T) {
	dir := t.TempDir()

	files, err := DiscoverEnvFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}
