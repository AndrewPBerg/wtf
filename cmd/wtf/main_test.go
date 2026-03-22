package main

import (
	"fmt"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/cli"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
)

func TestFormatError_Integration(t *testing.T) {
	// Verify that FormatError is accessible from main and handles all typed errors.
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"not a repo", cli.ErrNotARepo, "not in a git repo"},
		{"worktree not found", fmt.Errorf("%w: %q", git.ErrWorktreeNotFound, "x"), "couldn't find that worktree"},
		{"main worktree", git.ErrMainWorktree, "cannot remove main worktree"},
		{"invalid branch", fmt.Errorf("%w: %q", git.ErrInvalidBranchName, "x"), "invalid branch name"},
		{"generic", fmt.Errorf("oops"), "oops"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := cli.FormatError(tt.err)
			assert.Contains(t, msg, tt.contains)
		})
	}
}
