package cli

import (
	"bytes"
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [shell]",
	Short: "Print shell functions and completions for wtf integration",
	Long: `Print shell functions and tab completions to stdout for the given shell.

Add this to your shell profile to enable shell integration:

  # bash — add to ~/.bashrc
  eval "$(wtf init bash)"

  # zsh — add to ~/.zshrc
  eval "$(wtf init zsh)"

  # fish — add to ~/.config/fish/config.fish
  wtf init fish | source

Or run 'wtf setup shell' to configure this automatically.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var override string
		if len(args) > 0 {
			override = args[0]
		}
		return runInit(cmd, override, setup.NewShellDetector())
	},
}

func runInit(cmd *cobra.Command, override string, detector *setup.ShellDetector) error {
	shell, err := detector.Detect(override)
	if err != nil {
		return err
	}

	cr := func(s setup.Shell) (string, error) {
		return generateCompletion(cmd.Root(), s)
	}

	output := setup.Render(shell, setup.DefaultFuncs(), cr)
	_, err = fmt.Fprint(cmd.OutOrStdout(), output)
	return err
}

// generateCompletion renders the cobra completion script for the given shell.
func generateCompletion(root *cobra.Command, shell setup.Shell) (string, error) {
	var buf bytes.Buffer
	switch shell {
	case setup.Bash:
		err := root.GenBashCompletionV2(&buf, true)
		return buf.String(), err
	case setup.Zsh:
		err := root.GenZshCompletion(&buf)
		return buf.String(), err
	case setup.Fish:
		err := root.GenFishCompletion(&buf, true)
		return buf.String(), err
	default:
		return "", fmt.Errorf("unsupported shell for completions: %q", shell)
	}
}
