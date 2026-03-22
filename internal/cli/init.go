package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [shell]",
	Short: "Print shell functions for wtf integration",
	Long: `Print shell functions to stdout for the given shell.

Add this to your shell profile to enable shell integration:

  # bash/zsh — add to ~/.bashrc or ~/.zshrc
  eval "$(wtf init)"

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

	output := setup.Render(shell, setup.DefaultFuncs())
	_, err = fmt.Fprint(cmd.OutOrStdout(), output)
	return err
}
