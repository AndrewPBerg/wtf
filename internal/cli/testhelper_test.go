package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/stretchr/testify/require"
)

// initCLITestRepo creates a temporary git repo for CLI tests.
func initCLITestRepo(t *testing.T) string {
	t.Helper()
	// CLI creation commands persist identity state through WTF_HOME. Keep every
	// fixture's store private so tests cannot claim production state or each
	// other's workspace names.
	// Always replace the inherited home: callers may run tests with a real
	// WTF_HOME, and fixture creation must never write into it.
	t.Setenv("WTF_HOME", t.TempDir())

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

// testForge is a test double for the forge.Forge interface.
type testForge struct {
	prs  []forge.PR
	name string
}

func (f *testForge) Name() string { return f.name }
func (f *testForge) PRURL(n int) string {
	return fmt.Sprintf("https://github.com/test/repo/pull/%d", n)
}
func (f *testForge) FetchRef(n int) string {
	return fmt.Sprintf("pull/%d/head:pr-%d", n, n)
}
func (f *testForge) ListPRs(_ context.Context) ([]forge.PR, error) {
	return f.prs, nil
}
func (f *testForge) GetPR(_ context.Context, number int) (*forge.PR, error) {
	for i := range f.prs {
		if f.prs[i].Number == number {
			return &f.prs[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

// stubFetchManager wraps a real manager but makes FetchRefspec a no-op, so tests
// exercise worktree creation without contacting a remote.
type stubFetchManager struct {
	vcs.Manager
}

func (s *stubFetchManager) FetchRefspec(_, _, _ string) error { return nil }

// stubFailFetchManager wraps a real manager but fails every fetch.
type stubFailFetchManager struct {
	vcs.Manager
}

func (s *stubFailFetchManager) FetchRefspec(_, _, _ string) error {
	return fmt.Errorf("fetch failed: remote not found")
}
