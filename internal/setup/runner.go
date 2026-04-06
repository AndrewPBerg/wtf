package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/mattn/go-isatty"
)

// CmdExecutor abstracts shell command execution for testability.
type CmdExecutor interface {
	RunShell(dir, command string) error
	RunInteractive(dir, command string) error
}

// RealCmdExecutor executes shell commands via /bin/sh.
type RealCmdExecutor struct{}

// RunShell executes a command string via /bin/sh in the given directory.
// When stdout is a terminal, output is displayed in a fixed-height scrolling
// region so long builds don't flood the screen.
func (r *RealCmdExecutor) RunShell(dir, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir

	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		sw := ui.NewScrollWriter(os.Stdout, ui.DefaultScrollHeight)
		cmd.Stdout = sw
		cmd.Stderr = sw
		err := cmd.Run()
		sw.Flush()
		return err
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunInteractive executes a command with full terminal access (stdin/stdout/stderr).
// Uses the user's login shell with -ic so aliases and shell functions are available.
func (r *RealCmdExecutor) RunInteractive(dir, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.Command(shell, "-ic", command)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Runner orchestrates project setup.
type Runner struct {
	CmdExec    CmdExecutor
	EnvHandler *EnvFileHandler
}

// NewRunner creates a Runner with real implementations.
func NewRunner() *Runner {
	return &Runner{
		CmdExec:    &RealCmdExecutor{},
		EnvHandler: NewEnvFileHandler(),
	}
}

// Options controls which setup steps run after worktree creation.
type Options struct {
	SkipEnv     bool   // skip env file handling
	SkipInstall bool   // skip package manager install
	EnvStrategy string // "symlink" (default), "copy", "none"
}

// RunSetup runs the setup flow for a new worktree.
// By default: symlink env files from mainDir → targetDir, then auto-detect
// package manager and run install.
func (r *Runner) RunSetup(mainDir, targetDir string, opts Options) error {
	// 1. Handle env files
	if !opts.SkipEnv {
		strategy := opts.EnvStrategy
		if strategy == "" {
			strategy = "symlink"
		}
		if err := r.EnvHandler.HandleEnvFiles(mainDir, targetDir, strategy, nil); err != nil {
			return fmt.Errorf("handling env files: %w", err)
		}
	}

	// 2. Auto-detect and install
	if !opts.SkipInstall {
		if err := r.runAutoSetup(targetDir); err != nil {
			return fmt.Errorf("auto setup: %w", err)
		}
	}

	return nil
}

// RunHooks runs a list of hook commands in the given directory.
// Hooks run interactively with full terminal access, bypassing the scroll writer.
func (r *Runner) RunHooks(commands []string, dir string) error {
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if err := r.CmdExec.RunInteractive(dir, cmd); err != nil {
			return fmt.Errorf("running hook %q: %w", cmd, err)
		}
	}
	return nil
}

// runAutoSetup auto-detects package manager and runs install.
func (r *Runner) runAutoSetup(dir string) error {
	pm, err := DetectPackageManager(dir)
	if err != nil {
		return fmt.Errorf("detecting package manager: %w", err)
	}
	if pm == nil {
		return nil
	}

	if err := r.CmdExec.RunShell(dir, pm.InstallCmd); err != nil {
		return fmt.Errorf("running %s: %w", pm.InstallCmd, err)
	}

	return nil
}
