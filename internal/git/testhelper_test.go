package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// initTestRepo creates a temporary git repo with an initial commit and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	exec := &RealExecutor{}

	_, err := exec.Run(dir, "init", "-b", "main")
	require.NoError(t, err)

	_, err = exec.Run(dir, "config", "user.email", "test@test.com")
	require.NoError(t, err)

	_, err = exec.Run(dir, "config", "user.name", "Test")
	require.NoError(t, err)

	createCommit(t, dir, "initial commit")
	return dir
}

// createCommit creates an empty commit in the given repo.
func createCommit(t *testing.T, dir, msg string) {
	t.Helper()

	exec := &RealExecutor{}

	// Create a file so the commit is not empty
	f := filepath.Join(dir, msg+".txt")
	require.NoError(t, os.WriteFile(f, []byte(msg), 0o644))

	_, err := exec.Run(dir, "add", ".")
	require.NoError(t, err)

	_, err = exec.Run(dir, "commit", "-m", msg)
	require.NoError(t, err)
}
