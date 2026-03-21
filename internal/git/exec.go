package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Executor abstracts git CLI calls for testability.
type Executor interface {
	// Run executes a git command in the given directory and returns stdout.
	Run(dir string, args ...string) (string, error)
}

// RealExecutor shells out to the git CLI.
type RealExecutor struct{}

// Run executes a git command in the given directory and returns stdout.
func (r *RealExecutor) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
