package cli

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
)

// getRepoDir returns the top-level directory of the current git repo.
func getRepoDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	exec := &git.RealExecutor{}
	dir, err := exec.Run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	// Auto-register repo — fire-and-forget, never block commands
	_ = config.Add(dir)

	return dir, nil
}
