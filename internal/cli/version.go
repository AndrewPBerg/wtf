package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "0.8.0"

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of wtf",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]string{"version": Version})
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", bold("wtf"), cyan(Version))
		return nil
	},
}
