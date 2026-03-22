package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranchManager_Exists(t *testing.T) {
	dir := initTestRepo(t)
	bm := NewBranchManager(&RealExecutor{})

	exists, err := bm.Exists(dir, "main")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = bm.Exists(dir, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestBranchManager_CurrentBranch(t *testing.T) {
	dir := initTestRepo(t)
	bm := NewBranchManager(&RealExecutor{})

	branch, err := bm.CurrentBranch(dir)
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestBranchManager_MergedBranches(t *testing.T) {
	dir := initTestRepo(t)
	exec := &RealExecutor{}
	bm := NewBranchManager(exec)

	// Create a feature branch, add a commit, then merge it into main.
	_, err := exec.Run(dir, "checkout", "-b", "merged-branch")
	require.NoError(t, err)
	_, err = exec.Run(dir, "commit", "--allow-empty", "-m", "feature work")
	require.NoError(t, err)
	_, err = exec.Run(dir, "checkout", "main")
	require.NoError(t, err)
	_, err = exec.Run(dir, "merge", "--no-ff", "merged-branch", "-m", "merge merged-branch")
	require.NoError(t, err)

	branches, err := bm.MergedBranches(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, branches, "merged-branch")
}

func TestBranchManager_MergedBranches_ExcludesSameCommit(t *testing.T) {
	dir := initTestRepo(t)
	exec := &RealExecutor{}
	bm := NewBranchManager(exec)

	// Create a branch at the same commit as main — should NOT be considered merged.
	_, err := exec.Run(dir, "branch", "fresh-branch")
	require.NoError(t, err)

	branches, err := bm.MergedBranches(dir, "main")
	require.NoError(t, err)
	assert.NotContains(t, branches, "fresh-branch")
}

func TestBranchManager_ValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{name: "valid simple", branch: "feature-auth", wantErr: false},
		{name: "valid with slash", branch: "feature/auth", wantErr: false},
		{name: "empty", branch: "", wantErr: true},
		{name: "double dot", branch: "bad..name", wantErr: true},
		{name: "space", branch: "bad name", wantErr: true},
		{name: "tilde", branch: "bad~name", wantErr: true},
		{name: "caret", branch: "bad^name", wantErr: true},
		{name: "colon", branch: "bad:name", wantErr: true},
		{name: "ends with .lock", branch: "bad.lock", wantErr: true},
		{name: "dot in middle", branch: "feature.auth", wantErr: false},
		{name: "single valid", branch: "main", wantErr: false},
	}

	bm := NewBranchManager(&RealExecutor{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bm.ValidateBranchName(tt.branch)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidBranchName)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBranchManager_RemoteBranches(t *testing.T) {
	dir := initTestRepo(t)
	exec := &RealExecutor{}
	bm := NewBranchManager(exec)

	// Create a remote by cloning into a bare repo, then adding it as a remote.
	bareDir := t.TempDir()
	_, err := exec.Run(".", "clone", "--bare", dir, bareDir)
	require.NoError(t, err)
	_, err = exec.Run(dir, "remote", "add", "origin", bareDir)
	require.NoError(t, err)
	_, err = exec.Run(dir, "fetch", "origin")
	require.NoError(t, err)

	branches, err := bm.RemoteBranches(dir)
	require.NoError(t, err)
	assert.Contains(t, branches, "main")
}

func TestBranchManager_MergedBranches_ExcludesBase(t *testing.T) {
	dir := initTestRepo(t)
	bm := NewBranchManager(&RealExecutor{})

	branches, err := bm.MergedBranches(dir, "main")
	require.NoError(t, err)
	assert.NotContains(t, branches, "main")
}
