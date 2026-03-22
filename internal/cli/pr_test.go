package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// stubFetchExecutor wraps a real executor but stubs out fetch commands.
type stubFetchExecutor struct {
	real git.Executor
}

func (s *stubFetchExecutor) Run(dir string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "fetch" {
		return "", nil // stub fetch
	}
	return s.real.Run(dir, args...)
}

func TestResolvePR_ByNumber(t *testing.T) {
	f := &testForge{
		name: "github",
		prs: []forge.PR{
			{Number: 1, Title: "First", Branch: "feat-1"},
			{Number: 42, Title: "Answer", Branch: "the-answer"},
		},
	}

	pr, err := resolvePR(context.Background(), f, "42")
	require.NoError(t, err)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "Answer", pr.Title)
}

func TestResolvePR_ByNumberWithHash(t *testing.T) {
	f := &testForge{
		name: "github",
		prs:  []forge.PR{{Number: 7, Title: "Lucky", Branch: "lucky"}},
	}

	pr, err := resolvePR(context.Background(), f, "#7")
	require.NoError(t, err)
	assert.Equal(t, 7, pr.Number)
}

func TestResolvePR_ByBranch(t *testing.T) {
	f := &testForge{
		name: "github",
		prs: []forge.PR{
			{Number: 1, Title: "First", Branch: "feat-1"},
			{Number: 2, Title: "Second", Branch: "feat-2"},
		},
	}

	pr, err := resolvePR(context.Background(), f, "feat-2")
	require.NoError(t, err)
	assert.Equal(t, 2, pr.Number)
}

func TestResolvePR_BySubstring(t *testing.T) {
	f := &testForge{
		name: "github",
		prs: []forge.PR{
			{Number: 1, Title: "Add auth", Branch: "feature/add-auth"},
			{Number: 2, Title: "Fix bug", Branch: "fix/login-bug"},
		},
	}

	pr, err := resolvePR(context.Background(), f, "add-auth")
	require.NoError(t, err)
	assert.Equal(t, 1, pr.Number)
}

func TestResolvePR_MultipleBranchMatches(t *testing.T) {
	f := &testForge{
		name: "github",
		prs: []forge.PR{
			{Number: 1, Title: "Auth v1", Branch: "feature/auth-v1"},
			{Number: 2, Title: "Auth v2", Branch: "feature/auth-v2"},
		},
	}

	_, err := resolvePR(context.Background(), f, "auth")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple PRs match")
}

func TestResolvePR_NotFound(t *testing.T) {
	f := &testForge{
		name: "github",
		prs:  []forge.PR{},
	}

	_, err := resolvePR(context.Background(), f, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no open PR found")
}

func TestResolvePR_NumberNotFound(t *testing.T) {
	f := &testForge{
		name: "github",
		prs:  []forge.PR{},
	}

	_, err := resolvePR(context.Background(), f, "999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PR #999 not found")
}

func TestPRBranchName(t *testing.T) {
	tests := []struct {
		forge  string
		number int
		want   string
	}{
		{"github", 42, "pr-42"},
		{"gitlab", 10, "mr-10"},
		{"unknown", 1, "pr-1"},
	}

	for _, tt := range tests {
		t.Run(tt.forge, func(t *testing.T) {
			assert.Equal(t, tt.want, prBranchName(tt.forge, tt.number))
		})
	}
}

func TestRunPR_Integration(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	// Add a fake remote
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)
	// Create the local ref that fetch would create (pr-1)
	_, err = realExec.Run(dir, "checkout", "-b", "pr-1")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)

	// Use stubbed executor that skips real fetch
	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)
	cmd := prCmd

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	tf := &testForge{
		name: "github",
		prs: []forge.PR{
			{
				Number:    1,
				Title:     "Test PR",
				Branch:    "pr-branch",
				Author:    "tester",
				CreatedAt: time.Now(),
				URL:       "https://github.com/test/repo/pull/1",
			},
		},
	}

	ff := func(_ string) (forge.Forge, error) {
		return tf, nil
	}

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	err = runPR(cmd, "1", wm, exec, nil, ff)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Checked out")
}

func TestRunPR_ForgeDetectionError(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://bitbucket.org/test/repo.git")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)
	cmd := prCmd

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	ff := func(_ string) (forge.Forge, error) {
		return nil, fmt.Errorf("unsupported forge")
	}

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	err = runPR(cmd, "1", wm, exec, nil, ff)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "detecting forge") || strings.Contains(err.Error(), "unsupported"))
}
