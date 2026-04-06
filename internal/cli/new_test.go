package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "test-feature", newBase, wm, nil, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Created worktree at")
	assert.Contains(t, output, "test-feature")

	// Verify worktree was actually created
	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 2)
}

func TestNewCommand_InvalidBase(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "nonexistent-branch"
	defer func() { newBase = "main" }()

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "new-feature", newBase, wm, nil, false)
	assert.Error(t, err)
}

func TestNewCommand_InvalidBranchName(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "bad..name", newBase, wm, nil, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrInvalidBranchName)
}

func TestNewCommand_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "new-feature", newBase, wm, nil, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestNewCommand_WithRunner(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)
	newBase = "main"

	mock := &mockSetupExecutor{}
	runner := &setup.Runner{
		CmdExec:    mock,
		EnvHandler: setup.NewEnvFileHandler(),
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "setup-test", newBase, wm, runner, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
}

func TestNewCommand_SetupFailureIsWarning(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Create a .env file to trigger symlink during setup
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644))

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)
	newBase = "main"
	newNoSetup = false
	newNoEnv = false
	newNoInstall = false

	runner := &setup.Runner{
		CmdExec: &mockSetupExecutor{},
		EnvHandler: &setup.EnvFileHandler{
			Symlink: func(_, _ string) error { return fmt.Errorf("symlink broken") },
		},
	}

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "warn-test", newBase, wm, runner, false)
	// Should succeed — setup failures are warnings
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
	assert.Contains(t, stderr.String(), "setup failed")
}

// --- Branch flag tests ---

func TestNewBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	// Create a branch to simulate a remote branch
	_, err := realExec.Run(dir, "checkout", "-b", "remote-feature")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	// Delete the local branch so fetch can "create" it
	_, err = realExec.Run(dir, "branch", "-D", "remote-feature")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)

	// Re-create the branch since stubFetchExecutor won't actually fetch
	_, err = realExec.Run(dir, "branch", "remote-feature")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)

	err = runNewBranch(cmd, "remote-feature", wm, exec, nil, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created worktree at")
}

func TestNewBranch_InvalidName(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &stubFetchExecutor{real: &git.RealExecutor{}}
	wm := git.NewWorktreeManager(exec)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)

	err := runNewBranch(cmd, "bad..name", wm, exec, nil, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, git.ErrInvalidBranchName)
}

// --- PR resolution tests ---

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

func TestResolvePR_ByTitle(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		prs     []forge.PR
		wantNum int
		wantErr string
	}{
		{
			name:    "title substring match",
			arg:     "authentication",
			prs:     []forge.PR{{Number: 1, Title: "Add authentication system", Branch: "feat-1"}},
			wantNum: 1,
		},
		{
			name:    "case insensitive title",
			arg:     "FIX LOGIN",
			prs:     []forge.PR{{Number: 2, Title: "fix login bug", Branch: "fix-2"}},
			wantNum: 2,
		},
		{
			name: "title match only when no branch match",
			arg:  "auth",
			prs: []forge.PR{
				{Number: 1, Title: "Add auth middleware", Branch: "feat-auth"},
			},
			wantNum: 1, // matches branch substring first
		},
		{
			name: "multiple title matches",
			arg:  "auth",
			prs: []forge.PR{
				{Number: 1, Title: "Add auth", Branch: "feat-1"},
				{Number: 2, Title: "Fix auth", Branch: "feat-2"},
			},
			wantErr: "multiple PRs match",
		},
		{
			name:    "no title match",
			arg:     "zzz-nonexistent",
			prs:     []forge.PR{{Number: 1, Title: "Add feature", Branch: "feat-1"}},
			wantErr: "no open PR found",
		},
		{
			name:    "fuzzy title match",
			arg:     "ad authn",
			prs:     []forge.PR{{Number: 1, Title: "Add authentication", Branch: "feat-1"}},
			wantNum: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &testForge{name: "github", prs: tt.prs}
			pr, err := resolvePR(context.Background(), f, tt.arg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantNum, pr.Number)
			}
		})
	}
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

func TestRunNewPR_Integration(t *testing.T) {
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
	cmd := newCmd

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

	err = runNewPR(cmd, "1", wm, exec, nil, ff, false)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Checked out")
}

func TestRunNewPR_ForgeDetectionError(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://bitbucket.org/test/repo.git")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)
	cmd := newCmd

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

	err = runNewPR(cmd, "1", wm, exec, nil, ff, false)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "detecting forge") || strings.Contains(err.Error(), "unsupported"))
}

// --- Dispatch/mutual exclusivity tests ---

func TestNewCmd_NoMode(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Reset flags
	newBranchFlag = ""
	newPRFlag = ""
	defer func() { newBranchFlag = ""; newPRFlag = "" }()

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a branch name")
}

func TestNewCmd_BranchAndPositionalExclusive(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	newBranchFlag = "some-branch"
	newPRFlag = ""
	defer func() { newBranchFlag = ""; newPRFlag = "" }()

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{"my-branch"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestNewCmd_PRAndPositionalExclusive(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	newBranchFlag = ""
	newPRFlag = "42"
	defer func() { newBranchFlag = ""; newPRFlag = "" }()

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	err := cmd.RunE(cmd, []string{"my-branch"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// --- Auto-detect PR number tests ---

func TestDispatchNew_NumericArgRoutesPR(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)

	// Reset flags so only the positional arg "42" is active
	newBranchFlag = ""
	newPRFlag = ""
	defer func() { newBranchFlag = ""; newPRFlag = "" }()

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	// Numeric positional arg should route to PR path. The error will come
	// from the forge/API layer (not branch creation), proving the routing worked.
	err = dispatchNew(cmd, []string{"42"}, "main", "", "", false)
	require.Error(t, err)
	// Error must reference the PR number — proof it took the PR path
	assert.Contains(t, err.Error(), "PR #42")
}

func TestDispatchNew_HashNumericArgRoutesPR(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	// "#7" should also be detected as a PR number
	err = dispatchNew(cmd, []string{"#7"}, "main", "", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PR #7")
}

func TestDispatchNew_NonNumericArgCreatesBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := newCmd
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(new(bytes.Buffer))

	// Non-numeric arg should create a branch, not attempt PR checkout
	err := dispatchNew(cmd, []string{"feature-branch"}, "main", "", "", false)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Created worktree at")
}

func TestDispatchNew_ZeroIsNotPR(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	// "0" should NOT be treated as a PR (n > 0 guard)
	err := dispatchNew(cmd, []string{"0"}, "main", "", "", false)
	// Should go to branch creation path, which will fail on invalid branch
	// name or succeed — either way it should NOT hit the forge path
	if err != nil {
		assert.NotContains(t, err.Error(), "detecting forge")
	}
}

func TestDispatchNew_NegativeIsNotPR(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	// "-1" is not a valid PR number — should go to branch creation path
	err := dispatchNew(cmd, []string{"-1"}, "main", "", "", false)
	if err != nil {
		assert.NotContains(t, err.Error(), "detecting forge")
	}
}
