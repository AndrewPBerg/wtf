package cli

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/workspace"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(upCmd, downCmd) }

var upCmd = &cobra.Command{Use: "up <profile> <instance>", Short: "Materialize and start a profile environment", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error { return runUp(cmd, args[0], args[1]) }}
var downCmd = &cobra.Command{Use: "down <instance>", Short: "Stop a profile environment and release its resources", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return runDown(cmd, args[0]) }}

func runUp(cmd *cobra.Command, profile, instance string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	config, root, err := workspace.Load(dir)
	if err != nil {
		return err
	}
	state, err := workspace.Up(config, root, profile, instance)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), state)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Started %s (%s)\n", cyan(instance), cyan(profile))
	return err
}
func runDown(cmd *cobra.Command, instance string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	_, root, err := workspace.Load(dir)
	if err != nil {
		return err
	}
	if err := workspace.Down(root, instance); err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]string{"instance": instance, "status": "stopped"})
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s\n", cyan(instance))
	return err
}
