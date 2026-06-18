package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var (
	setupEnvOnly     bool
	setupInstallOnly bool
	setupCopyEnv     bool
)

func init() {
	setupCmd.Flags().BoolVar(&setupEnvOnly, "env", false, "Only handle env files")
	setupCmd.Flags().BoolVar(&setupInstallOnly, "install", false, "Only run package install")
	setupCmd.Flags().BoolVar(&setupCopyEnv, "copy-env", false, "Copy env files instead of symlinking (safer for agent worktrees)")
	setupCmd.MarkFlagsMutuallyExclusive("install", "copy-env")
	setupCmd.AddCommand(setupShellCmd)
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run project setup in current worktree",
	Long: `Runs project setup steps for the current worktree.

Handles env files from the main worktree and auto-detects the package
manager to run install. By default env files are symlinked; use --copy-env
to copy them for isolated agent worktrees.

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

	// Get the main worktree dir for env file resolution
	exec := &git.RealExecutor{}
	mainDir, err := exec.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("finding main worktree: %w", err)
	}
	mainPath := parseMainWorktreePath(mainDir)

	envStrategy := "symlink"
	if setupCopyEnv {
		envStrategy = "copy"
	}

	if setupEnvOnly {
		envFiles, dErr := setup.DiscoverEnvFiles(mainPath)
		if dErr != nil {
			return fmt.Errorf("discovering env files: %w", dErr)
		}
		handled, hErr := runner.EnvHandler.HandleEnvFiles(mainPath, dir, envStrategy, envFiles)
		if hErr != nil {
			return fmt.Errorf("handling env files: %w", hErr)
		}
		if len(handled) == 0 {
			_, _ = fmt.Fprintf(out, "%s No env files found\n", yellow("⚠"))
		} else {
			for _, f := range handled {
				_, _ = fmt.Fprintf(out, "%s %s %s\n", greenBold("✔"), f, envStrategyPastTense(envStrategy))
			}
		}
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

	runner.Out = out

	opts := setup.Options{EnvStrategy: envStrategy}
	if err := runner.RunSetup(mainPath, dir, opts); err != nil {
		return fmt.Errorf("running setup: %w", err)
	}

	_, _ = fmt.Fprintf(out, "%s Setup complete\n", greenBold("✔"))
	return nil
}

func envStrategyPastTense(strategy string) string {
	if strategy == "copy" {
		return "copied"
	}
	return "symlinked"
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
