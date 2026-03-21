package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwCommand(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "feature-switch", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)

	err = runSw(cmd, "switch", wm)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "feature-switch")
}

func TestSwCommand_NoMatch(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)

	err := runSw(cmd, "nonexistent", wm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no worktree found")
}
