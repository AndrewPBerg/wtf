package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/vcs"
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
	// dir names the repo to operate on, so inherited GIT_DIR/GIT_INDEX_FILE and
	// friends must not override it. git sets those for every hook it runs.
	cmd.Env = vcs.SanitizedEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
