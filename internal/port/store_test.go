package port

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_LoadNonexistent(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))
	m, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestFileStore_SaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wtf", "ports.json")
	store := NewFileStore(path)

	original := map[string]int{
		"main":        8080,
		"feature/foo": 8081,
	}
	require.NoError(t, store.Save(original))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, original, loaded)
}

func TestFileStore_SaveCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "ports.json")
	store := NewFileStore(path)

	require.NoError(t, store.Save(map[string]int{"main": 3000}))
	assert.FileExists(t, path)
}

func TestFileStore_LoadCorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	require.NoError(t, os.WriteFile(path, []byte("not json{"), 0o644))

	store := NewFileStore(path)
	_, err := store.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing port store")
}

func TestFileStore_LoadEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	store := NewFileStore(path)
	_, err := store.Load()
	assert.Error(t, err)
}

func TestFileStore_SaveToReadOnlyDir(t *testing.T) {
	// Use /dev/null as parent — can't create subdirectories
	store := NewFileStore("/dev/null/ports.json")
	err := store.Save(map[string]int{"main": 8000})
	assert.Error(t, err)
}

func TestFileStore_LoadUnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"main":3000}`), 0o644))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	store := NewFileStore(path)
	_, err := store.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading port store")
}
