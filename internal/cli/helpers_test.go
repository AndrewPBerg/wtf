package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
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
	assert.True(t, errors.Is(err, ErrNotARepo))
}

func TestFormatError_NotARepo(t *testing.T) {
	msg := FormatError(ErrNotARepo)
	assert.Contains(t, msg, "not in a git repo")
	assert.Contains(t, msg, "git init")
}

func TestFormatError_WorktreeNotFound(t *testing.T) {
	err := fmt.Errorf("%w: %q", git.ErrWorktreeNotFound, "my-branch")
	msg := FormatError(err)
	assert.Contains(t, msg, "couldn't find that worktree")
	assert.Contains(t, msg, "wtf ls")
}

func TestFormatError_MultipleMatches(t *testing.T) {
	err := fmt.Errorf("%w %q: a, b", git.ErrMultipleMatches, "feat")
	msg := FormatError(err)
	assert.Contains(t, msg, "multiple worktrees match")
}

func TestFormatError_BranchAlreadyExists(t *testing.T) {
	err := fmt.Errorf("%w: my-branch", git.ErrBranchAlreadyExists)
	msg := FormatError(err)
	assert.Contains(t, msg, "already exists")
}

func TestFormatError_InvalidBranchName(t *testing.T) {
	err := fmt.Errorf("%w: %q", git.ErrInvalidBranchName, "bad..name")
	msg := FormatError(err)
	assert.Contains(t, msg, "invalid branch name")
	assert.Contains(t, msg, "valid git refs")
}

func TestFormatError_MainWorktree(t *testing.T) {
	msg := FormatError(git.ErrMainWorktree)
	assert.Contains(t, msg, "cannot remove main worktree")
	assert.Contains(t, msg, "managed by git")
}

func TestFormatError_MissingArgs(t *testing.T) {
	err := fmt.Errorf("accepts 1 arg(s), received 0")
	msg := FormatError(err)
	assert.Contains(t, msg, "missing required argument")
	assert.Contains(t, msg, "wtf --help")
}

func TestFormatError_TooManyArgs(t *testing.T) {
	err := fmt.Errorf("accepts 1 arg(s), received 3")
	msg := FormatError(err)
	assert.Contains(t, msg, "too many arguments")
	assert.Contains(t, msg, "expected 1")
	assert.Contains(t, msg, "got 3")
}

func TestFormatError_UnknownCommand(t *testing.T) {
	err := fmt.Errorf(`unknown command "foo" for "wtf"`)
	msg := FormatError(err)
	assert.Contains(t, msg, "is not a wtf command")
	assert.Contains(t, msg, "wtf --help")
}

func TestFormatError_UnknownCommandSuggestion(t *testing.T) {
	err := fmt.Errorf(`unknown command "swe" for "wtf"`)
	msg := FormatError(err)
	assert.Contains(t, msg, "is not a wtf command")
	assert.Contains(t, msg, "Did you mean")
	assert.Contains(t, msg, "sw")
}

func TestFormatError_UnknownFlag(t *testing.T) {
	err := fmt.Errorf("unknown flag: --foo")
	msg := FormatError(err)
	assert.Contains(t, msg, "unknown flag: --foo")
	assert.Contains(t, msg, "wtf --help")
}

func TestFormatError_GenericError(t *testing.T) {
	err := fmt.Errorf("something went wrong")
	msg := FormatError(err)
	assert.Contains(t, msg, "something went wrong")
}
