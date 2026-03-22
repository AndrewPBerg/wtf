package cli

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update wtf to the latest version",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUpdate(cmd)
	},
}

const installURL = "https://raw.githubusercontent.com/AndrewPBerg/wtf/main/install.sh"

// execCommand is the function used to run external commands. Override in tests.
var execCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func runUpdate(cmd *cobra.Command) error {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Updating wtf...\n", cyan("⟳"))

	out, err := execCommand("sh", "-c", fmt.Sprintf("curl -fsSL %s | sh", installURL))
	if err != nil {
		return fmt.Errorf("updating wtf: %w\n%s", err, out)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Updated successfully.\n", greenBold("✔"))
	return nil
}
