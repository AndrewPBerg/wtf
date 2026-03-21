package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/require"
)

// initCLITestRepo creates a temporary git repo for CLI tests.
func initCLITestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	exec := &git.RealExecutor{}

	_, err := exec.Run(dir, "init", "-b", "main")
	require.NoError(t, err)

	_, err = exec.Run(dir, "config", "user.email", "test@test.com")
	require.NoError(t, err)

	_, err = exec.Run(dir, "config", "user.name", "Test")
	require.NoError(t, err)

	f := filepath.Join(dir, "init.txt")
	require.NoError(t, os.WriteFile(f, []byte("init"), 0o644))

	_, err = exec.Run(dir, "add", ".")
	require.NoError(t, err)

	_, err = exec.Run(dir, "commit", "-m", "initial commit")
	require.NoError(t, err)

	return dir
}
