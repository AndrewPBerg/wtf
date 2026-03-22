package cli

import (
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteWorktrees(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a worktree so there's something to complete
	_, err := wm.Add(dir, "feature-a", "main")
	require.NoError(t, err)

	completions, directive := completeWorktrees(nil, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "main")
	assert.Contains(t, completions, "feature-a")
}

func TestCompleteWorktrees_NotARepo(t *testing.T) {
	t.Chdir(t.TempDir())

	completions, directive := completeWorktrees(nil, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Empty(t, completions)
}

func TestCompleteRemoteBranches(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}

	// Create a remote by cloning into a bare repo
	bareDir := t.TempDir()
	_, err := exec.Run(".", "clone", "--bare", dir, bareDir)
	require.NoError(t, err)
	_, err = exec.Run(dir, "remote", "add", "origin", bareDir)
	require.NoError(t, err)

	// Create a remote-only branch
	_, err = exec.Run(dir, "checkout", "-b", "remote-only")
	require.NoError(t, err)
	_, err = exec.Run(dir, "push", "origin", "remote-only")
	require.NoError(t, err)
	_, err = exec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	// Delete local branch so it only exists on remote
	_, err = exec.Run(dir, "branch", "-D", "remote-only")
	require.NoError(t, err)
	// Fetch to make sure remote tracking is up to date
	_, err = exec.Run(dir, "fetch", "origin")
	require.NoError(t, err)

	completions, directive := completeRemoteBranches(nil, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "remote-only")
	// "main" already has a worktree, so it should be excluded
	assert.NotContains(t, completions, "main")
}

func TestCompleteCleanTargets(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	exec := &git.RealExecutor{}
	wm := git.NewWorktreeManager(exec)

	// Create a branch, add a commit, merge it — then create a worktree for it
	_, err := exec.Run(dir, "checkout", "-b", "merged-branch")
	require.NoError(t, err)
	_, err = exec.Run(dir, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "merged-branch", "-m", "merge merged-branch")
	require.NoError(t, err)

	// Now create a worktree for the merged branch (so clean can find it)
	_, err = wm.Add(dir, "merged-branch", "merged-branch")
	require.NoError(t, err)

	completions, directive := completeCleanTargets(nil, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Should include merged-branch with a "merged" annotation
	found := false
	for _, c := range completions {
		if c == "merged-branch\tmerged" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected merged-branch in completions, got: %v", completions)
}

func TestCompleteCleanTargets_NothingToClean(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	completions, directive := completeCleanTargets(nil, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Empty(t, completions)
}

func TestFilterPrefix(t *testing.T) {
	tests := []struct {
		name   string
		items  []string
		prefix string
		want   []string
	}{
		{"empty prefix returns all", []string{"a", "b", "c"}, "", []string{"a", "b", "c"}},
		{"matching prefix", []string{"apple", "avocado", "banana"}, "a", []string{"apple", "avocado"}},
		{"no matches", []string{"apple", "banana"}, "z", nil},
		{"nil items", nil, "a", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterPrefix(tt.items, tt.prefix)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompleteRegisteredRepos(t *testing.T) {
	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1, repo2})

	completions, directive := completeRegisteredRepos(nil, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Should include both name\tpath entries and raw paths
	assert.GreaterOrEqual(t, len(completions), 4) // 2 name entries + 2 path entries
}

func TestCompleteRegisteredRepos_NoRepos(t *testing.T) {
	setupGlobalRegistry(t, []string{})

	completions, directive := completeRegisteredRepos(nil, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Empty(t, completions)
}
