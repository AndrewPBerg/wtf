package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of wtf",
	Run: func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", bold("wtf"), cyan(Version))
	},
}
