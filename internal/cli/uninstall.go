package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var uninstallForce bool

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the wtf binary from the system",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUninstall(cmd)
	},
}

func runUninstall(cmd *cobra.Command) error {
	binPath, err := findBinary()
	if err != nil {
		return err
	}

	if !uninstallForce {
		ok := confirmPrompt(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Remove %s? [y/N] ", binPath))
		if !ok {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	if err := os.Remove(binPath); err != nil {
		return fmt.Errorf("removing %s: %w", binPath, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", binPath)
	return nil
}

func confirmPrompt(in io.Reader, out io.Writer, prompt string) bool {
	_, _ = fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}

func findBinary() (string, error) {
	path, err := exec.LookPath("wtf")
	if err != nil {
		return "", fmt.Errorf("wtf binary not found in PATH: %w", err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	return abs, nil
}
