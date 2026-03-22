package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRunUpdate_Success(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	execCommand = func(_ string, _ ...string) ([]byte, error) {
		return []byte("installed"), nil
	}

	buf := new(bytes.Buffer)
	cmd := updateCmd
	cmd.SetOut(buf)

	err := runUpdate(cmd)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Updating wtf")
	assert.Contains(t, output, "Updated successfully")
}

func TestRunUpdate_Failure(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	execCommand = func(_ string, _ ...string) ([]byte, error) {
		return []byte("connection refused"), fmt.Errorf("exit status 1")
	}

	buf := new(bytes.Buffer)
	cmd := updateCmd
	cmd.SetOut(buf)

	err := runUpdate(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "updating wtf")
	assert.Contains(t, err.Error(), "connection refused")
}
