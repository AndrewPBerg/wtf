package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
)

// CmdExecutor abstracts shell command execution for testability.
type CmdExecutor interface {
	RunShell(dir, command string) error
}

// RealCmdExecutor executes shell commands via /bin/sh.
type RealCmdExecutor struct{}

// RunShell executes a command string via /bin/sh in the given directory.
func (r *RealCmdExecutor) RunShell(dir, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
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

// RunSetup runs the full setup flow for a worktree.
// If cfg is nil, auto-detect package manager and run install if found.
// If cfg exists: handle env files → auto-detect + run install → evaluate & run setup steps → run hooks.
func (r *Runner) RunSetup(cfg *config.ProjectConfig, mainDir, targetDir, branch string) error {
	if cfg == nil {
		return r.runAutoSetup(targetDir)
	}

	// 1. Handle env files
	strategy := cfg.Env.Strategy
	files := cfg.Env.Files
	if err := r.EnvHandler.HandleEnvFiles(mainDir, targetDir, strategy, files); err != nil {
		return fmt.Errorf("handling env files: %w", err)
	}

	// 2. Auto-detect and install
	if err := r.runAutoSetup(targetDir); err != nil {
		return fmt.Errorf("auto setup: %w", err)
	}

	// 3. Run setup steps
	ctx := &ConditionContext{
		Branch: branch,
		Dir:    targetDir,
		GetEnv: os.Getenv,
	}

	for _, step := range cfg.Setup {
		if step.If != "" {
			ok, err := EvalCondition(step.If, ctx)
			if err != nil {
				return fmt.Errorf("evaluating condition for step %q: %w", step.Name, err)
			}
			if !ok {
				continue
			}
		}

		if err := r.CmdExec.RunShell(targetDir, step.Run); err != nil {
			return fmt.Errorf("running setup step %q: %w", step.Name, err)
		}
	}

	// 4. Run on_create hooks
	if err := r.RunHooks(cfg.Hooks.OnCreate, targetDir); err != nil {
		return fmt.Errorf("running on_create hooks: %w", err)
	}

	return nil
}

// RunHooks runs a list of hook commands in the given directory.
func (r *Runner) RunHooks(commands []string, dir string) error {
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if err := r.CmdExec.RunShell(dir, cmd); err != nil {
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
