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
	// Error must come from the forge/PR path — not from branch creation.
	// With gh installed: "PR #42 not found"; without: "detecting forge: ..."
	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "PR #42") || strings.Contains(errMsg, "detecting forge"),
		"expected PR-path error, got: %s", errMsg)
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
	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "PR #7") || strings.Contains(errMsg, "detecting forge"),
		"expected PR-path error, got: %s", errMsg)
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

// --- setupOptsFromFlags tests ---

func TestSetupOptsFromFlags(t *testing.T) {
	tests := []struct {
		name        string
		noSetup     bool
		noEnv       bool
		noInstall   bool
		wantSkipEnv bool
		wantSkipPM  bool
	}{
		{"defaults", false, false, false, false, false},
		{"no-setup skips all", true, false, false, true, true},
		{"no-env only", false, true, false, true, false},
		{"no-install only", false, false, true, false, true},
		{"no-env and no-install", false, true, true, true, true},
		{"no-setup overrides individual", true, true, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newNoSetup = tt.noSetup
			newNoEnv = tt.noEnv
			newNoInstall = tt.noInstall
			defer func() { newNoSetup = false; newNoEnv = false; newNoInstall = false }()

			opts := setupOptsFromFlags()
			assert.Equal(t, tt.wantSkipEnv, opts.SkipEnv)
			assert.Equal(t, tt.wantSkipPM, opts.SkipInstall)
		})
	}
}

// --- dispatchNew --base exclusivity tests ---

func TestDispatchNew_BaseWithBranchFlagErrors(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	require.NoError(t, cmd.Flags().Set("base", "develop"))
	defer func() { _ = cmd.Flags().Set("base", "main") }()

	err := dispatchNew(cmd, nil, "develop", "some-branch", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--base cannot be used with --branch")
}

func TestDispatchNew_BaseWithPRFlagErrors(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	require.NoError(t, cmd.Flags().Set("base", "develop"))
	defer func() { _ = cmd.Flags().Set("base", "main") }()

	err := dispatchNew(cmd, nil, "develop", "", "42", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--base cannot be used with --pr")
}

func TestDispatchNew_BaseWithNumericArgErrors(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	require.NoError(t, cmd.Flags().Set("base", "develop"))
	defer func() { _ = cmd.Flags().Set("base", "main") }()

	err = dispatchNew(cmd, []string{"42"}, "develop", "", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--base cannot be used with a PR number")
}

// --- JSON output tests ---

func TestNewBranch_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "checkout", "-b", "json-branch")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "branch", "-D", "json-branch")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)

	_, err = realExec.Run(dir, "branch", "json-branch")
	require.NoError(t, err)

	jsonOutput = true
	defer func() { jsonOutput = false }()

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))

	err = runNewBranch(cmd, "json-branch", wm, exec, nil, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, `"path"`)
	assert.Contains(t, out, `"branch"`)
	assert.Contains(t, out, `"json-branch"`)
}

func TestRunNewPR_JSON(t *testing.T) {
	dir := initCLITestRepo(t)

	realExec := &git.RealExecutor{}
	_, err := realExec.Run(dir, "remote", "add", "origin", "https://github.com/test/repo.git")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "-b", "pr-5")
	require.NoError(t, err)
	_, err = realExec.Run(dir, "checkout", "main")
	require.NoError(t, err)

	exec := &stubFetchExecutor{real: realExec}
	wm := git.NewWorktreeManager(exec)

	jsonOutput = true
	defer func() { jsonOutput = false }()

	var stdout, stderr bytes.Buffer
	cmd := newCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	tf := &testForge{
		name: "github",
		prs: []forge.PR{
			{Number: 5, Title: "JSON PR", Branch: "json-pr", Author: "tester"},
		},
	}
	ff := func(_ string) (forge.Forge, error) { return tf, nil }

	t.Chdir(dir)

	err = runNewPR(cmd, "5", wm, exec, nil, ff, false)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, `"path"`)
	assert.Contains(t, out, `"branch"`)
	assert.Contains(t, out, `"pr"`)
	assert.Contains(t, out, `"number"`)
	assert.Contains(t, out, `"JSON PR"`)
}

func TestRunNewPR_NoRemote(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &stubFetchExecutor{real: &git.RealExecutor{}}
	wm := git.NewWorktreeManager(exec)

	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	ff := func(_ string) (forge.Forge, error) { return &testForge{name: "github"}, nil }

	err := runNewPR(cmd, "1", wm, exec, nil, ff, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote URL")
}

// --- Clean JSON tests ---

func TestCleanCommand_DryRun_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	wtPath, err := wm.Add(dir, "json-merged", "main")
	require.NoError(t, err)
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "json-merged", "-m", "merge json-merged")
	require.NoError(t, err)

	jsonOutput = true
	cleanDryRun = true
	cleanForce = false
	defer func() { jsonOutput = false; cleanDryRun = false }()

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, `"dry_run"`)
	assert.Contains(t, out, `"json-merged"`)
	assert.Contains(t, out, `"merged"`)
}

func TestCleanCommand_Remove_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	wtPath, err := wm.Add(dir, "json-rm", "main")
	require.NoError(t, err)
	_, err = exec.Run(wtPath, "commit", "--allow-empty", "-m", "feature")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "json-rm", "-m", "merge json-rm")
	require.NoError(t, err)

	jsonOutput = true
	cleanDryRun = false
	cleanForce = true
	defer func() { jsonOutput = false; cleanForce = false }()

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))

	err = runClean(cmd, wm, exec)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, `"json-rm"`)
	assert.Contains(t, out, `"merged"`)
}

// --- runNewBranch fetch error ---

func TestNewBranch_FetchError(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	failExec := &stubFailFetchExecutor{real: &git.RealExecutor{}}
	wm := git.NewWorktreeManager(failExec)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))

	err := runNewBranch(cmd, "some-branch", wm, failExec, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching remote branch")
}

// --- runPostCreateSetup edge cases ---

func TestRunPostCreateSetup_NilRunner(_ *testing.T) {
	cmd := newCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	// Should not panic with nil runner.
	runPostCreateSetup(cmd, wm, nil, "/nonexistent", "/nonexistent")
}

func TestRunPostCreateSetup_MainWorktreeError(t *testing.T) {
	dir := t.TempDir() // not a git repo
	cmd := newCmd
	stderr := new(bytes.Buffer)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(stderr)

	runner := &setup.Runner{
		CmdExec:    &mockSetupExecutor{},
		EnvHandler: setup.NewEnvFileHandler(),
	}
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	runPostCreateSetup(cmd, wm, runner, dir, dir)
	assert.Contains(t, stderr.String(), "setup skipped")
}

// --- newOutputWriters tests ---

func TestNewOutputWriters_SwitchMode(t *testing.T) {
	cmd := newCmd
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	msgW, pathW := newOutputWriters(cmd, true)
	assert.Equal(t, stderr, msgW, "in switch mode, messages go to stderr")
	assert.Equal(t, stdout, pathW, "in switch mode, path goes to stdout")
}

func TestNewOutputWriters_NormalMode(t *testing.T) {
	cmd := newCmd
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))

	msgW, pathW := newOutputWriters(cmd, false)
	assert.Equal(t, stdout, msgW, "in normal mode, messages go to stdout")
	assert.Nil(t, pathW, "in normal mode, pathW is nil")
}
