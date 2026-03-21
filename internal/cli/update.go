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

func runUpdate(cmd *cobra.Command) error {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Updating wtf...\n", cyan("⟳"))

	out, err := exec.Command("sh", "-c", fmt.Sprintf("curl -fsSL %s | sh", installURL)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating wtf: %w\n%s", err, out)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Updated successfully.\n", greenBold("✔"))
	return nil
}
