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

	// Create a branch that is merged (same commit as main)
	_, err := exec.Run(dir, "branch", "merged-branch")
	require.NoError(t, err)

	branches, err := bm.MergedBranches(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, branches, "merged-branch")
}

func TestBranchManager_MergedBranches_ExcludesBase(t *testing.T) {
	dir := initTestRepo(t)
	bm := NewBranchManager(&RealExecutor{})

	branches, err := bm.MergedBranches(dir, "main")
	require.NoError(t, err)
	assert.NotContains(t, branches, "main")
}
