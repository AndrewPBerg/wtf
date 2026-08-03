package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []Worktree
		errMsg string
	}{
		{
			name:  "empty output",
			input: "",
			want:  nil,
		},
		{
			name: "single main worktree",
			input: `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

`,
			want: []Worktree{
				{Path: "/home/user/repo", VCS: vcs.KindGit, Head: "abc123", Branch: "main", IsMain: true},
			},
		},
		{
			name: "main and feature worktree",
			input: `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

worktree /home/user/repo--feature-auth
HEAD def456
branch refs/heads/feature/auth

`,
			want: []Worktree{
				{Path: "/home/user/repo", VCS: vcs.KindGit, Head: "abc123", Branch: "main", IsMain: true},
				{Path: "/home/user/repo--feature-auth", VCS: vcs.KindGit, Head: "def456", Branch: "feature/auth"},
			},
		},
		{
			name: "detached head",
			input: `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

worktree /home/user/repo--detached
HEAD def456
detached

`,
			want: []Worktree{
				{Path: "/home/user/repo", VCS: vcs.KindGit, Head: "abc123", Branch: "main", IsMain: true},
				{Path: "/home/user/repo--detached", VCS: vcs.KindGit, Head: "def456", IsDetached: true},
			},
		},
		{
			name: "bare repo",
			input: `worktree /home/user/repo.git
HEAD abc123
bare

`,
			want: []Worktree{
				{Path: "/home/user/repo.git", VCS: vcs.KindGit, Head: "abc123", IsBare: true, IsMain: true},
			},
		},
		{
			name: "prunable worktree",
			input: `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

worktree /home/user/repo--gone
HEAD def456
branch refs/heads/gone
prunable gitdir file points to non-existent location

`,
			want: []Worktree{
				{Path: "/home/user/repo", VCS: vcs.KindGit, Head: "abc123", Branch: "main", IsMain: true},
				{Path: "/home/user/repo--gone", VCS: vcs.KindGit, Head: "def456", Branch: "gone", Prunable: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWorktreeList(tt.input)
			if tt.errMsg != "" {
				assert.ErrorContains(t, err, tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorktreePath(t *testing.T) {
	tests := []struct {
		name     string
		mainPath string
		branch   string
		want     string
	}{
		{
			name:     "simple branch",
			mainPath: "/code/myrepo",
			branch:   "feature",
			want:     "/code/feature--myrepo",
		},
		{
			name:     "branch with slash",
			mainPath: "/code/myrepo",
			branch:   "feature/auth",
			want:     "/code/feature-auth--myrepo",
		},
		{
			name:     "nested slashes",
			mainPath: "/code/myrepo",
			branch:   "feature/auth/login",
			want:     "/code/feature-auth-login--myrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WorktreePath(tt.mainPath, tt.branch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorktreeManager_List_Integration(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	wts, err := wm.List(dir)
	require.NoError(t, err)
	require.Len(t, wts, 1)
	assert.Equal(t, dir, wts[0].Path)
	assert.Equal(t, "main", wts[0].Branch)
	assert.True(t, wts[0].IsMain)
}

func TestWorktreeManager_AddAndList_Integration(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	wtPath, err := wm.Add(dir, "test-branch", "main")
	require.NoError(t, err)
	assert.Contains(t, wtPath, "test-branch--")

	wts, err := wm.List(dir)
	require.NoError(t, err)
	require.Len(t, wts, 2)
	assert.Equal(t, "test-branch", wts[1].Branch)
}

func TestWorktreeManager_Find_Integration(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "feature-login", "main")
	require.NoError(t, err)

	wt, err := wm.Find(dir, "login")
	require.NoError(t, err)
	assert.Equal(t, "feature-login", wt.Branch)
}

func TestWorktreeManager_Find_NoMatch(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Find(dir, "nonexistent")
	assert.ErrorIs(t, err, ErrWorktreeNotFound)
}

func TestWorktreeManager_Find_MultipleMatches(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "feature-a", "main")
	require.NoError(t, err)
	_, err = wm.Add(dir, "feature-b", "main")
	require.NoError(t, err)

	_, err = wm.Find(dir, "feature")
	assert.ErrorIs(t, err, ErrMultipleMatches)
}

func TestWorktreeManager_Find_ExactMatchPreferred(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "test", "main")
	require.NoError(t, err)
	_, err = wm.Add(dir, "testd", "main")
	require.NoError(t, err)

	wt, err := wm.Find(dir, "test")
	require.NoError(t, err)
	assert.Equal(t, "test", wt.Branch)
}

func TestWorktreeManager_Remove_Integration(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	_, err := wm.Add(dir, "to-remove", "main")
	require.NoError(t, err)

	err = wm.Remove(dir, "to-remove", "/somewhere-else", false)
	require.NoError(t, err)

	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 1)

	bm := NewBranchManager(&RealExecutor{})
	exists, err := bm.Exists(dir, "to-remove")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestWorktreeManager_Remove_DirtyWorktree(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	wtPath, err := wm.Add(dir, "dirty-branch", "main")
	require.NoError(t, err)

	// Create an untracked file in the worktree to make it dirty
	err = os.WriteFile(filepath.Join(wtPath, "untracked.txt"), []byte("dirty"), 0o644)
	require.NoError(t, err)

	err = wm.Remove(dir, "dirty-branch", "/somewhere-else", false)
	assert.ErrorIs(t, err, ErrWorktreeHasChanges)

	// Force remove should work
	err = wm.Remove(dir, "dirty-branch", "/somewhere-else", true)
	assert.NoError(t, err)
}

func TestWorktreeManager_Remove_Main(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	err := wm.Remove(dir, "main", "/somewhere-else", false)
	assert.ErrorIs(t, err, ErrMainWorktree)
}

func TestWorktreeManager_MainWorktree(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	main, err := wm.MainWorktree(dir)
	require.NoError(t, err)
	assert.True(t, main.IsMain)
	assert.Equal(t, "main", main.Branch)
}

func TestManagerKind(t *testing.T) {
	wm := NewWorktreeManager(&RealExecutor{})
	assert.Equal(t, vcs.KindGit, wm.Kind())
}

func TestStateDir(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	stateDir, err := wm.StateDir(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".git", "wtf"), stateDir)

	// Every worktree of a repo must agree on one state dir, since wtf keeps
	// allocated ports and forge caches there.
	wtPath, err := wm.Add(dir, "feat", "main")
	require.NoError(t, err)

	fromWt, err := wm.StateDir(wtPath)
	require.NoError(t, err)
	assert.Equal(t, stateDir, fromWt)
}

func TestStateDirError(t *testing.T) {
	wm := NewWorktreeManager(&RealExecutor{})
	_, err := wm.StateDir(t.TempDir())
	assert.Error(t, err)
}

func TestCurrentRef(t *testing.T) {
	dir := initTestRepo(t)
	wm := NewWorktreeManager(&RealExecutor{})

	ref, err := wm.CurrentRef(dir)
	require.NoError(t, err)
	assert.Equal(t, "main", ref)

	wtPath, err := wm.Add(dir, "feat/auth", "main")
	require.NoError(t, err)

	ref, err = wm.CurrentRef(wtPath)
	require.NoError(t, err)
	assert.Equal(t, "feat/auth", ref)
}

func TestCurrentRefError(t *testing.T) {
	wm := NewWorktreeManager(&RealExecutor{})
	_, err := wm.CurrentRef(t.TempDir())
	assert.Error(t, err)
}

func TestCleanable(t *testing.T) {
	dir := initTestRepo(t)
	exec := &RealExecutor{}
	wm := NewWorktreeManager(exec)

	// An unmerged worktree is not cleanable.
	wtPath, err := wm.Add(dir, "feat", "main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, "f.txt"), []byte("x"), 0o644))
	_, err = exec.Run(wtPath, "add", ".")
	require.NoError(t, err)
	_, err = exec.Run(wtPath, "commit", "-m", "work")
	require.NoError(t, err)

	got, err := wm.Cleanable(dir)
	require.NoError(t, err)
	assert.Empty(t, got, "unmerged work must never be reported as cleanable")

	// Once merged into main, it is.
	_, err = exec.Run(dir, "merge", "--no-ff", "-m", "merge", "feat")
	require.NoError(t, err)

	got, err = wm.Cleanable(dir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "feat", got[0].Branch)
	assert.False(t, got[0].IsMain, "the main worktree is never cleanable")
}

func TestCleanableError(t *testing.T) {
	wm := NewWorktreeManager(&RealExecutor{})
	_, err := wm.Cleanable(t.TempDir())
	assert.Error(t, err)
}
