package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "update" {
			found = true
			break
		}
	}
	assert.True(t, found, "update command should be registered")
}

func TestUpdateCommandMetadata(t *testing.T) {
	assert.Equal(t, "update", updateCmd.Use)
	assert.Contains(t, updateCmd.Short, "Update")
}
