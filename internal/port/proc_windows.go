//go:build windows

package port

import (
	"os"
	"os/exec"
)

// setSysProcAttr is a no-op on Windows — Setpgid is not supported.
func setSysProcAttr(_ *exec.Cmd) {}

// killProcessGroup kills the process by PID on Windows.
// Process groups work differently on Windows so we just kill the main process.
func killProcessGroup(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = p.Kill()
}
