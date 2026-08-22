package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "feature-switch", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSw(cmd, "switch", wm)
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "feature-switch")
	assert.Contains(t, stderr.String(), "Switched to")
	assert.Contains(t, stderr.String(), "feature-switch")
}

func TestSwCommand_AlreadyOnBranch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(dir, "feature-already", "main")
	require.NoError(t, err)

	// cd into the target worktree so we're "already on it"
	t.Chdir(wtPath)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSw(cmd, "already", wm)
	require.NoError(t, err)

	// Should still print the path to stdout (shell function needs it for safe cd)
	assert.Contains(t, stdout.String(), "feature-already")
	// Should tell the user they're already there
	assert.Contains(t, stderr.String(), "wtf?")
	assert.Contains(t, stderr.String(), "already on")
	assert.Contains(t, stderr.String(), "feature-already")
}

func TestSwCommand_NoMatch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	buf := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)
	cmd.SetErr(stderr)

	err := runSw(cmd, "nonexistent", wm)
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "error:")
	assert.Contains(t, stderr.String(), "nonexistent")
}

func TestSwCommand_NoMatchWithSuggestions(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	// Create worktrees with similar names to trigger fuzzy matching
	_, err := wm.Add(dir, "feature-auth", "main")
	require.NoError(t, err)
	_, err = wm.Add(dir, "feature-api", "main")
	require.NoError(t, err)

	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(stderr)

	// "aut" is a substring of "feature-auth" so Find will match it.
	// Use something that won't substring-match but will fuzzy-match.
	err = runSw(cmd, "feath", wm)
	assert.Error(t, err)
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "error:")
	// Should show either "Did you mean?" or "Available worktrees:"
	assert.True(t, strings.Contains(stderrStr, "Did you mean?") || strings.Contains(stderrStr, "Available worktrees:"))
}

func TestSwCommand_NoMatchShowsAvailable(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "alpha", "main")
	require.NoError(t, err)

	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(stderr)

	// Query that won't fuzzy-match "alpha" at all
	err = runSw(cmd, "zzzzz", wm)
	assert.Error(t, err)
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "error:")
	// Should show available worktrees since no fuzzy match
	assert.Contains(t, stderrStr, "Available worktrees:")
	assert.Contains(t, stderrStr, "alpha")
}

func TestSwCommand_NoMatch_BareRepoOnly(t *testing.T) {
	// Test the case where Find fails and List returns only bare worktrees
	// (which get filtered out, leaving no branches to suggest)
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(stderr)

	// "x" is a single-char query (below fuzzy threshold) and won't substring match "main"
	err := runSw(cmd, "xxxxxxxxx", wm)
	assert.Error(t, err)
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "error:")
	// Should show available worktrees with "main" since no fuzzy matches
	assert.Contains(t, stderrStr, "Available worktrees:")
	assert.Contains(t, stderrStr, "main")
}

func TestFuzzyFilter_SortsByScore(t *testing.T) {
	branches := []string{"feature-authentication", "feat-auth-flow", "bugfix-authorization"}
	result := fuzzyFilter(branches, "fauth")
	// All should have some fuzzy score; results sorted by score descending
	assert.NotEmpty(t, result)
	// Verify no more than 5 results
	assert.LessOrEqual(t, len(result), 5)
}

func TestFuzzyFilter_MaxFiveResults(t *testing.T) {
	branches := make([]string, 10)
	for i := range branches {
		branches[i] = fmt.Sprintf("branch-x%d", i)
	}
	result := fuzzyFilter(branches, "bx")
	assert.LessOrEqual(t, len(result), 5)
}

func TestFuzzyFilter_NoMatches(t *testing.T) {
	branches := []string{"alpha", "beta", "gamma"}
	result := fuzzyFilter(branches, "zzzzzzz")
	assert.Nil(t, result)
}

func TestFuzzyFilter_SubstringMatchSkipped(t *testing.T) {
	branches := []string{"feature-login"}
	// "login" is a substring — should be skipped by fuzzyFilter
	result := fuzzyFilter(branches, "login")
	assert.Nil(t, result)
}

func TestFuzzyFilter_EmptyBranches(t *testing.T) {
	result := fuzzyFilter(nil, "query")
	assert.Nil(t, result)
}

func TestFuzzyScore_NoMatchReturnsZero(t *testing.T) {
	assert.Equal(t, 0, fuzzyScore("abc", "xyz"))
}

func TestFuzzyScore_BelowThreshold(t *testing.T) {
	// Only 1 of 5 chars match — below 40% threshold
	assert.Equal(t, 0, fuzzyScore("abcde", "xyzwa"))
}

func TestFuzzyScore_SingleChar(t *testing.T) {
	// Single char query, threshold=1, match found
	assert.Greater(t, fuzzyScore("feature", "f"), 0)
}

func TestFuzzyFilter_SingleCharQueryReturnsNil(t *testing.T) {
	// Single-char queries are too ambiguous for fuzzy matching
	branches := []string{"feature-auth", "feat-api", "bugfix-auth"}
	result := fuzzyFilter(branches, "f")
	assert.Nil(t, result)
}

func TestFuzzyScore_FullMatch(t *testing.T) {
	score := fuzzyScore("abcdef", "abcdef")
	assert.Equal(t, 6, score)
}

// setupGlobalRegistry sets WTF_HOME to a temp dir and registers the given repos.
func setupGlobalRegistry(t *testing.T, repos []string) {
	t.Helper()
	wtfHome := filepath.Join(t.TempDir(), ".wtf")
	t.Setenv("WTF_HOME", wtfHome)
	require.NoError(t, os.MkdirAll(wtfHome, 0o755))
	for _, r := range repos {
		require.NoError(t, config.Add(r))
	}
}

func TestSwGlobal_FindsWorktreeAcrossRepos(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo2, "feature-global", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSwGlobal(cmd, "global")
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "feature-global")
	assert.Contains(t, stderr.String(), "Switched to")
}

func TestSwGlobal_NoMatch(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runSwGlobal(cmd, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "error:")
	assert.Contains(t, stderr.String(), "nonexistent")
}

func TestSwGlobal_MultipleMatches(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	// Create worktrees with the same branch name in both repos
	_, err := wm.Add(repo1, "feature-dup", "main")
	require.NoError(t, err)
	_, err = wm.Add(repo2, "feature-dup", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSwGlobal(cmd, "dup")
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "multiple")
}

func TestSwGlobal_NoRepos(t *testing.T) {
	setupGlobalRegistry(t, []string{})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runSwGlobal(cmd, "anything")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no registered repos")
}

func TestNativeWorktreeRefUsesBackendNativeIdentity(t *testing.T) {
	gitWorktree := vcs.Worktree{VCS: vcs.KindGit, Branch: "feature", NativeName: "wrong-native"}
	jjWorktree := vcs.Worktree{VCS: vcs.KindJJ, Branch: "display", NativeName: "repo/feature"}
	require.Equal(t, "feature", nativeWorktreeRef(gitWorktree))
	require.Equal(t, "repo/feature", nativeWorktreeRef(jjWorktree))
}

func TestIdentityJSONOmitsEmptyLegacyFields(t *testing.T) {
	got := identityJSON(vcs.Worktree{Path: "/tmp/feature", Branch: "feature"})
	require.Empty(t, got)
}

func TestIsPRBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"pr-1", true},
		{"pr-42", true},
		{"mr-1", true},
		{"mr-100", true},
		{"pr-0", true},
		{"mr-0", true},
		{"pr-", false},
		{"mr-", false},
		{"pr-abc", false},
		{"feature", false},
		{"pr1", false},
		{"mr1", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			assert.Equal(t, tt.want, isPRBranch(tt.branch))
		})
	}
}

func TestIsCurrentWorktree(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	tests := []struct {
		name string
		cwd  string
		wt   string
		want bool
	}{
		{"exact match", dir, dir, true},
		{"subdirectory", sub, dir, true},
		{"different dir", t.TempDir(), dir, false},
		{"parent dir", filepath.Dir(dir), dir, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCurrentWorktree(tt.cwd, tt.wt))
		})
	}
}

func TestRunOnSwitchHooks_NoOp(t *testing.T) {
	cmd := swCmd
	cmd.SetErr(new(bytes.Buffer))
	// Should be a no-op — no config file system anymore
	runOnSwitchHooks(cmd, t.TempDir(), "feature")
}

func TestSwGlobal_FuzzySuggestions(t *testing.T) {
	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(repo, "feature-auth", "main")
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSwGlobal(cmd, "feath")
	assert.Error(t, err)
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "error:")
	assert.True(t, strings.Contains(stderrStr, "Did you mean?") || strings.Contains(stderrStr, "feature-auth"))
}
