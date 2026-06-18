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
	mock.on("worktree add /feat--repo feat", "", nil)

	wm := NewWorktreeManager(mock)
	path, err := wm.Add("/repo", "feat", "main")
	assert.NoError(t, err)
	assert.Equal(t, "/feat--repo", path)
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
	mock.on("worktree add -b feat /feat--repo main", "", fmt.Errorf("fail"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "feat", "main")
	assert.ErrorContains(t, err, "adding worktree")
}

func TestWorktreeManager_Add_BranchAlreadyCheckedOut(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list main", "  main", nil)
	mock.on("worktree add /main--repo main", "", fmt.Errorf("exit status 128: Preparing worktree (checking out 'main')\nfatal: 'main' is already used by worktree at '/repo'"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "main", "main")
	assert.ErrorIs(t, err, ErrBranchAlreadyInUse)
	assert.ErrorContains(t, err, "already used by worktree at /repo")
}

func TestWorktreeManager_Add_BranchAlreadyCheckedOutNoPath(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list main", "  main", nil)
	mock.on("worktree add /main--repo main", "", fmt.Errorf("is already used by worktree"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "main", "main")
	assert.ErrorIs(t, err, ErrBranchAlreadyInUse)
}

func TestWorktreeManager_Add_PathAlreadyExists(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n", nil)
	mock.on("branch --list feat", "", nil) // branch doesn't exist
	mock.on("worktree add -b feat /feat--repo main", "", fmt.Errorf("exit status 128: Preparing worktree (checking out 'feat')\nfatal: '/feat--repo' already exists"))

	wm := NewWorktreeManager(mock)
	_, err := wm.Add("/repo", "feat", "main")
	assert.ErrorIs(t, err, ErrPathAlreadyExists)
	assert.ErrorContains(t, err, "/feat--repo")
}

func TestWorktreeManager_Remove_ForceFlags(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "force-test", "main")
	assert.NoError(t, err)

	err = wm.Remove(dir, "force-test", "/somewhere-else", true)
	assert.NoError(t, err)
}

func TestWorktreeManager_Remove_FindError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "", fmt.Errorf("fail"))

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", "/somewhere-else", false)
	assert.ErrorContains(t, err, "finding worktree")
}

func TestWorktreeManager_Remove_RemoveError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /feat--repo\nHEAD def\nbranch refs/heads/feat\n", nil)
	mock.on("worktree remove /feat--repo", "", fmt.Errorf("dirty"))

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", "/somewhere-else", false)
	assert.ErrorContains(t, err, "removing worktree")
}

func TestWorktreeManager_Remove_DoesNotDeleteBranch(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /feat--repo\nHEAD def\nbranch refs/heads/feat\n", nil)
	mock.on("worktree remove /feat--repo", "", nil)

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", "/somewhere-else", false)
	assert.NoError(t, err)
}

func TestWorktreeManager_Remove_BlocksCurrentDir(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /feat--repo\nHEAD def\nbranch refs/heads/feat\n", nil)

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", "/feat--repo", false)
	assert.ErrorIs(t, err, ErrWorktreeIsCurrentDir)
}

func TestWorktreeManager_Remove_BlocksSubdirOfCurrentDir(t *testing.T) {
	mock := newMockExecutor()
	mock.on("worktree list --porcelain", "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\nworktree /feat--repo\nHEAD def\nbranch refs/heads/feat\n", nil)

	wm := NewWorktreeManager(mock)
	err := wm.Remove("/repo", "feat", "/feat--repo/src/main", false)
	assert.ErrorIs(t, err, ErrWorktreeIsCurrentDir)
}
