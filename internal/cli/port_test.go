package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortCommand_AllocatesPort(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := portCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runPort(cmd)
	require.NoError(t, err)

	port := strings.TrimSpace(stdout.String())
	assert.NotEmpty(t, port)
	// No framework indicators in test repo → default base 8000
	assert.Equal(t, "8000", port)
}

func TestPortCommand_Idempotent(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// First call
	stdout1 := new(bytes.Buffer)
	cmd1 := portCmd
	cmd1.SetOut(stdout1)
	cmd1.SetErr(new(bytes.Buffer))
	require.NoError(t, runPort(cmd1))

	// Second call
	stdout2 := new(bytes.Buffer)
	cmd2 := portCmd
	cmd2.SetOut(stdout2)
	cmd2.SetErr(new(bytes.Buffer))
	require.NoError(t, runPort(cmd2))

	assert.Equal(t, stdout1.String(), stdout2.String())
}

func TestPortCommand_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	jsonOutput = true
	defer func() { jsonOutput = false }()

	stdout := new(bytes.Buffer)
	cmd := portCmd
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))

	err := runPort(cmd)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, `"port"`)
	assert.Contains(t, out, `"branch"`)
	assert.Contains(t, out, "8000")
}
