package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecute(t *testing.T) {
	// Reset args to avoid test runner flags leaking in
	rootCmd.SetArgs([]string{})
	err := Execute()
	assert.NoError(t, err)
}

func TestRootCommandMetadata(t *testing.T) {
	assert.Equal(t, "wt", rootCmd.Use)
	assert.Contains(t, rootCmd.Short, "WorkTreeForage")
}
