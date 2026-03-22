package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var (
	setupEnvOnly     bool
	setupInstallOnly bool
)

func init() {
	setupCmd.Flags().BoolVar(&setupEnvOnly, "env", false, "Only handle env files")
	setupCmd.Flags().BoolVar(&setupInstallOnly, "install", false, "Only run package install")
	setupCmd.AddCommand(setupShellCmd)
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run project setup in current worktree",
	Long: `Runs project setup steps for the current worktree.

Auto-detects the package manager and runs install. If a .wt-forge.toml
config file exists, also handles env files and runs custom setup steps.

Use 'wtf setup shell' for shell integration setup.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runProjectSetup(cmd, setup.NewRunner())
	},
}

var setupShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Configure shell integration automatically",
	Long: `Detects your shell, finds the RC file, and appends the wtf init line.

This is a one-time setup that adds shell integration so commands like
'sw' work as native shell functions.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runSetupShell(cmd, setup.NewShellDetector(), nil)
	},
}

func runProjectSetup(cmd *cobra.Command, runner *setup.Runner) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()

	cfg, err := config.LoadProjectConfig(dir)
	if err != nil {
		return fmt.Errorf("loading project config: %w", err)
	}

	if cfg != nil {
		if err := config.ValidateProjectConfig(cfg); err != nil {
			return fmt.Errorf("invalid project config: %w", err)
		}
	}

	// Get the main worktree dir for env file resolution
	exec := &git.RealExecutor{}
	mainDir, err := exec.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("finding main worktree: %w", err)
	}
	mainPath := parseMainWorktreePath(mainDir)

	// Get current branch
	branch, _ := exec.Run(dir, "rev-parse", "--abbrev-ref", "HEAD")

	if setupEnvOnly {
		if cfg == nil {
			_, _ = fmt.Fprintf(out, "%s No .wt-forge.toml found, nothing to do\n", yellow("⚠"))
			return nil
		}
		if err := runner.EnvHandler.HandleEnvFiles(mainPath, dir, cfg.Env.Strategy, cfg.Env.Files); err != nil {
			return fmt.Errorf("handling env files: %w", err)
		}
		_, _ = fmt.Fprintf(out, "%s Env files handled\n", greenBold("✔"))
		return nil
	}

	if setupInstallOnly {
		pm, pmErr := setup.DetectPackageManager(dir)
		if pmErr != nil {
			return pmErr
		}
		if pm == nil {
			_, _ = fmt.Fprintf(out, "%s No package manager detected\n", yellow("⚠"))
			return nil
		}
		if err := runner.CmdExec.RunShell(dir, pm.InstallCmd); err != nil {
			return fmt.Errorf("running %s: %w", pm.InstallCmd, err)
		}
		_, _ = fmt.Fprintf(out, "%s Ran %s\n", greenBold("✔"), cyan(pm.InstallCmd))
		return nil
	}

	if err := runner.RunSetup(cfg, mainPath, dir, branch); err != nil {
		return fmt.Errorf("running setup: %w", err)
	}

	_, _ = fmt.Fprintf(out, "%s Setup complete\n", greenBold("✔"))
	return nil
}

func runSetupShell(cmd *cobra.Command, detector *setup.ShellDetector, rcm *setup.RCFileManager) error {
	out := cmd.OutOrStdout()

	shell, err := detector.Detect("")
	if err != nil {
		return err
	}

	if rcm == nil {
		rcm, err = setup.NewRCFileManager()
		if err != nil {
			return err
		}
	}

	rcPath, err := rcm.RCFilePath(shell)
	if err != nil {
		return err
	}

	present, err := setup.IsInitPresent(rcPath)
	if err != nil {
		return fmt.Errorf("checking rc file: %w", err)
	}

	if present {
		_, _ = fmt.Fprintf(out, "%s Shell integration already configured in %s\n", greenBold("✔"), cyan(rcPath))
		_, _ = fmt.Fprintf(out, "%s Tab completions included automatically via %s\n", greenBold("✔"), cyan("wtf init"))
		return nil
	}

	initLine := setup.InitLine(shell)
	_, _ = fmt.Fprintf(out, "%s  %s\n", bold("Detected shell:"), cyan(shell))
	_, _ = fmt.Fprintf(out, "%s         %s\n", bold("RC file:"), cyan(rcPath))
	_, _ = fmt.Fprintf(out, "%s        %s\n", bold("Will add:"), dim(initLine))
	_, _ = fmt.Fprint(out, bold("Proceed?")+" [y/N] ")

	reader := bufio.NewReader(cmd.InOrStdin())
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		_, _ = fmt.Fprintln(out, yellow("Aborted."))
		return nil
	}

	if err := setup.AppendInit(rcPath, shell); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "%s Added to %s. Restart your shell or run: %s\n", greenBold("✔"), cyan(rcPath), dim("source "+rcPath))
	_, _ = fmt.Fprintf(out, "%s Tab completions included automatically via %s\n", greenBold("✔"), cyan("wtf init"))
	return nil
}

// parseMainWorktreePath extracts the first worktree path from porcelain output.
func parseMainWorktreePath(porcelain string) string {
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			return strings.TrimPrefix(line, "worktree ")
		}
	}
	return ""
}
