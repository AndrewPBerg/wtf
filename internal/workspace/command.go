package workspace

import (
	"os"
	"os/exec"
)

// setupExecutor keeps lifecycle progress off stdout so --json remains valid.
type setupExecutor struct{}

func (setupExecutor) RunShell(dir, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (setupExecutor) RunInteractive(dir, command string) error {
	return setupExecutor{}.RunShell(dir, command)
}
