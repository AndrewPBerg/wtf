package cli

import (
	"bytes"
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
	assert.Equal(t, "wtf", rootCmd.Use)
	assert.Contains(t, rootCmd.Short, "WorkTreeForge")
}

func TestVersionFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--version"})

	err := Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), Version)
}
