package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealExecutor_GitVersion(t *testing.T) {
	exec := &RealExecutor{}
	out, err := exec.Run(".", "--version")
	require.NoError(t, err)
	assert.Contains(t, out, "git version")
}

func TestRealExecutor_InvalidDir(t *testing.T) {
	exec := &RealExecutor{}
	_, err := exec.Run("/nonexistent-dir-xyz", "status")
	assert.Error(t, err)
}

func TestRealExecutor_InvalidCommand(t *testing.T) {
	exec := &RealExecutor{}
	_, err := exec.Run(".", "not-a-real-command")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git not-a-real-command")
}
