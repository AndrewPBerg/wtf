//go:build windows

package workspace

import (
	"os"
	"os/exec"
)

func isolateProcess(_ *exec.Cmd) {}

func stopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
