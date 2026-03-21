package git

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorktreeManager_List_Error(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "", fmt.Errorf("not a repo"))

	wm := NewWorktreeManager(mock)
	_, err := wm.List("/some/dir")
	assert.ErrorContains(t, err, "listing worktrees")
}

func TestWorktreeManager_MainWorktree_NoWorktrees(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "", nil)

	wm := NewWorktreeManager(mock)
	_, err := wm.MainWorktree("/some/dir")
	assert.ErrorContains(t, err, "no worktrees found")
}

func TestWorktreeManager_Add_ExistingBranch(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list feat", "  feat", nil)
	mock.on("worktree add /repo--feat feat", "", nil)

	wm := NewWorktreeManager(mock)
	path, err := wm.Add("/repo", "feat", "main")
	assert.NoError(t, err)
	assert.Equal(t, "/repo--feat", path)
}

func TestWorktreeManager_Add_BranchCheckError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list feat", "", fmt.Errorf("fail"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "feat", "main")
	assert.ErrorContains(t, err, "checking branch")
}

func TestWorktreeManager_Add_WorktreeAddError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list feat", "", nil) // branch doesn't exist
	mock.on("worktree add -b feat /repo--feat main", "", fmt.Errorf("fail"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "feat", "main")
	assert.ErrorContains(t, err, "adding worktree")
}

func TestWorktreeManager_Remove_ForceFlags(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "force-test", "main")
	assert.NoError(t, err)

	err = wm.Remove(dir, "force-test", true)
	assert.NoError(t, err)
}

func TestWorktreeManager_Remove_FindError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "", fmt.Errorf("fail"))

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", false)
	assert.ErrorContains(t, err, "finding worktree")
}

func TestWorktreeManager_Remove_RemoveError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /repo--feat\nHEAD def\nbranch refs/heads/feat\n", nil)
	mock.on("worktree remove /repo--feat", "", fmt.Errorf("dirty"))

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", false)
	assert.ErrorContains(t, err, "removing worktree")
}

func TestWorktreeManager_Remove_BranchDeleteError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /repo--feat\nHEAD def\nbranch refs/heads/feat\n", nil)
	mock.on("worktree remove /repo--feat", "", nil)
	mock.on("branch -d feat", "", fmt.Errorf("not merged"))

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", false)
	assert.ErrorContains(t, err, "deleting branch")
}
