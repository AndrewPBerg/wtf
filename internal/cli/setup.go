package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure shell integration automatically",
	Long: `Detects your shell, finds the RC file, and appends the wtf init line.

This is a one-time setup that adds shell integration so commands like
'sw' work as native shell functions.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runSetup(cmd, setup.NewShellDetector(), nil)
	},
}

func runSetup(cmd *cobra.Command, detector *setup.ShellDetector, rcm *setup.RCFileManager) error {
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
	return nil
}
