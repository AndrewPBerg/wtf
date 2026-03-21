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

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runSw(cmd, "switch", wm)
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "feature-switch")
	assert.Contains(t, stderr.String(), "Switched to")
	assert.Contains(t, stderr.String(), "feature-switch")
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
