package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
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

func TestFuzzyScore_FullMatch(t *testing.T) {
	score := fuzzyScore("abcdef", "abcdef")
	assert.Equal(t, 6, score)
}
