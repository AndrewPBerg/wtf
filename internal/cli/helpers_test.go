package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRepoDir(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	got, err := getRepoDir()
	require.NoError(t, err)
	assert.Equal(t, dir, got)
}

func TestGetRepoDir_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := getRepoDir()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}
