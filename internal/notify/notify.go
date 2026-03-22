package notify

import (
	"context"
	"io"
	"os"
)

// Notifier sends user-facing notifications.
type Notifier interface {
	// Notify sends a notification with the given title and body.
	Notify(ctx context.Context, title, body string) error

	// Name returns the notifier backend name (e.g. "desktop", "terminal").
	Name() string
}

// Option configures a Notifier returned by New.
type Option func(*config)

type config struct {
	terminalOnly bool
	writer       io.Writer
	lookPath     func(string) (string, error)
	cmdRunner    cmdRunner
}

// WithTerminalOnly forces the terminal notifier even if desktop is available.
func WithTerminalOnly(v bool) Option {
	return func(c *config) {
		c.terminalOnly = v
	}
}

// WithWriter sets the output writer for the terminal notifier.
// Defaults to os.Stderr.
func WithWriter(w io.Writer) Option {
	return func(c *config) {
		c.writer = w
	}
}

// withLookPath overrides exec.LookPath for testing.
func withLookPath(fn func(string) (string, error)) Option {
	return func(c *config) {
		c.lookPath = fn
	}
}

// withCmdRunner overrides the command runner for testing.
func withCmdRunner(fn cmdRunner) Option {
	return func(c *config) {
		c.cmdRunner = fn
	}
}

// New returns the best available Notifier for the current platform.
// It tries desktop notifications first (notify-send on Linux, osascript on macOS),
// falling back to terminal output if neither is available.
func New(opts ...Option) Notifier {
	cfg := &config{
		writer: os.Stderr,
	}
	for _, o := range opts {
		o(cfg)
	}

	if !cfg.terminalOnly {
		if d := newDesktop(cfg); d != nil {
			return d
		}
	}

	return newTerminal(cfg.writer)
}
