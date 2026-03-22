package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
)

// ErrNotARepo is returned when the current directory is not inside a git repository.
var ErrNotARepo = errors.New("not a git repository")

// getRepoDir returns the top-level directory of the current git repo.
func getRepoDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	exec := &git.RealExecutor{}
	dir, err := exec.Run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotARepo
	}

	// Auto-register repo — fire-and-forget, never block commands
	_ = config.Add(dir)

	return dir, nil
}

// FormatError returns a user-friendly, WTF-themed error string.
func FormatError(err error) string {
	switch {
	case errors.Is(err, ErrNotARepo):
		return fmt.Sprintf(
			"%s you're not in a git repo\n  %s cd into a repo or run %s to get started",
			redBold("wtf?"),
			dim("hint:"),
			cyan("git init"),
		)

	case errors.Is(err, git.ErrWorktreeNotFound):
		return fmt.Sprintf(
			"%s couldn't find that worktree\n  %s run %s to see available worktrees",
			redBold("wtf?"),
			dim("hint:"),
			cyan("wtf ls"),
		)

	case errors.Is(err, git.ErrMultipleMatches):
		return fmt.Sprintf("%s %s", redBold("wtf?"), err.Error())

	case errors.Is(err, git.ErrBranchAlreadyExists):
		return fmt.Sprintf("%s %s", redBold("wtf?"), err.Error())

	case errors.Is(err, git.ErrPathAlreadyExists):
		return fmt.Sprintf(
			"%s something's already squatting at that path\n  %s %s\n  %s nuke it or pick a different name",
			redBold("wtf?"),
			dim("path:"),
			err.Error(),
			dim("hint:"),
		)

	case errors.Is(err, git.ErrInvalidBranchName):
		return fmt.Sprintf(
			"%s %s\n  %s branch names must be valid git refs",
			redBold("wtf?"),
			err.Error(),
			dim("hint:"),
		)

	case errors.Is(err, git.ErrMainWorktree):
		return fmt.Sprintf(
			"%s %s\n  %s the main worktree is managed by git directly",
			redBold("wtf?"),
			err.Error(),
			dim("hint:"),
		)

	default:
		return fmt.Sprintf("%s %s", redBold("wtf?"), err.Error())
	}
}
