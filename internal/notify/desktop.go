package notify

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// cmdRunner abstracts command execution for testing.
type cmdRunner func(ctx context.Context, name string, args ...string) error

func defaultCmdRunner(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// desktopNotifier sends native desktop notifications via shell commands.
type desktopNotifier struct {
	cmd string // "notify-send" or "osascript"
	run cmdRunner
}

// newDesktop returns a desktopNotifier if a suitable command is found, or nil.
func newDesktop(cfg *config) *desktopNotifier {
	lookPath := exec.LookPath
	if cfg.lookPath != nil {
		lookPath = cfg.lookPath
	}

	var cmd string
	switch runtime.GOOS {
	case "linux":
		if path, err := lookPath("notify-send"); err == nil && path != "" {
			cmd = "notify-send"
		}
	case "darwin":
		if path, err := lookPath("osascript"); err == nil && path != "" {
			cmd = "osascript"
		}
	}

	if cmd == "" {
		return nil
	}

	runner := defaultCmdRunner
	if cfg.cmdRunner != nil {
		runner = cfg.cmdRunner
	}

	return &desktopNotifier{cmd: cmd, run: runner}
}

func (d *desktopNotifier) Name() string { return "desktop" }

func (d *desktopNotifier) Notify(ctx context.Context, title, body string) error {
	switch d.cmd {
	case "notify-send":
		if err := d.run(ctx, "notify-send", "-a", "wtf", title, body); err != nil {
			return fmt.Errorf("sending desktop notification: %w", err)
		}
	case "osascript":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		if err := d.run(ctx, "osascript", "-e", script); err != nil {
			return fmt.Errorf("sending desktop notification: %w", err)
		}
	default:
		return fmt.Errorf("unknown desktop command: %s", d.cmd)
	}
	return nil
}
