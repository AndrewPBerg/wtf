package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [--shell bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate the autocompletion script for wtf.

By default, detects your current shell from $SHELL.
Use --shell to override.

Example:
  # Auto-detect and write to stdout
  wtf completion

  # Force a specific shell
  wtf completion --shell zsh`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		shell, _ := cmd.Flags().GetString("shell")
		if shell == "" {
			var err error
			shell, err = detectShell()
			if err != nil {
				return err
			}
		}

		install, _ := cmd.Flags().GetBool("install")
		if install {
			return runCompletionInstall(cmd, shell)
		}

		out := cmd.OutOrStdout()
		switch shell {
		case "bash":
			return cmd.Root().GenBashCompletionV2(out, true)
		case "zsh":
			return cmd.Root().GenZshCompletion(out)
		case "fish":
			return cmd.Root().GenFishCompletion(out, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(out)
		default:
			return fmt.Errorf("unsupported shell: %s (valid: bash, zsh, fish, powershell)", shell)
		}
	},
}

func init() {
	completionCmd.Flags().String("shell", "", "shell type: bash, zsh, fish, powershell")
	completionCmd.Flags().Bool("install", false, "install completion file to standard user-local path")
}

func runCompletionInstall(cmd *cobra.Command, shell string) error {
	setupShell, err := setup.ParseShellName(shell)
	if err != nil {
		return fmt.Errorf("--install does not support %q (supported: bash, zsh, fish)", shell)
	}

	var buf bytes.Buffer
	switch shell {
	case "bash":
		err = cmd.Root().GenBashCompletionV2(&buf, true)
	case "zsh":
		err = cmd.Root().GenZshCompletion(&buf)
	case "fish":
		err = cmd.Root().GenFishCompletion(&buf, true)
	default:
		return fmt.Errorf("--install does not support %q (supported: bash, zsh, fish)", shell)
	}
	if err != nil {
		return fmt.Errorf("generating completion script: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	path, err := setup.WriteCompletionFile(setupShell, home, buf.String())
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Installed completions to %s\n", greenBold("✔"), cyan(path))
	return nil
}

// detectShell reads $SHELL and returns a normalized shell name.
func detectShell() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "", fmt.Errorf("$SHELL is not set; use --shell to specify (bash, zsh, fish, powershell)")
	}
	base := filepath.Base(shell)
	// Handle versioned names like bash5, zsh5.9, etc.
	base = strings.TrimRight(base, "0123456789.")
	switch base {
	case "bash", "zsh", "fish":
		return base, nil
	case "pwsh", "powershell":
		return "powershell", nil
	default:
		return "", fmt.Errorf("unsupported shell %q from $SHELL; use --shell to specify (bash, zsh, fish, powershell)", shell)
	}
}
