package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)
	newBase = "main"

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNew(cmd, "test-feature", wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Created worktree at")
	assert.Contains(t, output, "test-feature")

	// Verify worktree was actually created
	wts, err := wm.List(dir)
	require.NoError(t, err)
	assert.Len(t, wts, 2)
}
