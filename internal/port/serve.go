package port

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ServeResult is returned after starting a dev server.
type ServeResult struct {
	PID     int
	Command string
	Port    int
	LogFile string
}

// StartDevServer launches the framework's dev server in the background.
// The server runs in dir with PORT set in the environment. Output is
// redirected to a log file (.wtf-server.log) and the PID is recorded
// in .wtf-server.pid. Returns nil if no framework is detected.
func StartDevServer(dir string, port int) (*ServeResult, error) {
	fw := DetectFramework(dir)
	if fw == nil {
		return nil, nil
	}

	// Expand $PORT in the dev command
	devCmd := strings.ReplaceAll(fw.DevCmd, "$PORT", strconv.Itoa(port))

	logPath := filepath.Join(dir, ".wtf-server.log")
	pidPath := filepath.Join(dir, ".wtf-server.pid")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("creating server log: %w", err)
	}

	cmd := exec.Command("sh", "-c", devCmd)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
	// Detach from parent process group so the server survives after wtf exits.
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("starting dev server: %w", err)
	}

	// Close the log file handle in the parent — the child owns it now.
	_ = logFile.Close()

	pid := cmd.Process.Pid

	// Record PID for cleanup
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o644)

	// Release the child process so we don't zombie it
	_ = cmd.Process.Release()

	return &ServeResult{
		PID:     pid,
		Command: devCmd,
		Port:    port,
		LogFile: logPath,
	}, nil
}

// StopDevServer kills the dev server in dir if a PID file exists.
func StopDevServer(dir string) error {
	pidPath := filepath.Join(dir, ".wtf-server.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading server pid: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidPath)
		return nil
	}

	// Kill the process group (negative PID) to catch child processes
	killProcessGroup(pid)

	_ = os.Remove(pidPath)
	_ = os.Remove(filepath.Join(dir, ".wtf-server.log"))
	return nil
}
