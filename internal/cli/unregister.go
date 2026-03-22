package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(unregisterCmd)
}

var unregisterCmd = &cobra.Command{
	Use:               "unregister [path]",
	Short:             "Remove a repo from the wtf registry",
	Long:              `Remove a repo from the wtf registry (~/.wtf/repos.json). If no path is given, unregisters the current repo.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRegisteredRepos,
	RunE: func(cmd *cobra.Command, args []string) error {
		var repoPath string

		if len(args) == 1 {
			abs, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}
			repoPath = abs
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			// Find the git root for the current directory
			dir, err := getRepoDirWithoutRegister(cwd)
			if err != nil {
				return err
			}
			repoPath = dir
		}

		removed, err := config.Remove(repoPath)
		if err != nil {
			return fmt.Errorf("unregistering repo: %w", err)
		}

		if !removed {
			return fmt.Errorf("repo %s is not registered", repoPath)
		}

		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{
				"unregistered": repoPath,
			})
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Unregistered %s\n", greenBold("✔"), cyan(repoPath))
		return nil
	},
}

// getRepoDirWithoutRegister returns the git root without auto-registering.
func getRepoDirWithoutRegister(cwd string) (string, error) {
	exec := &git.RealExecutor{}
	dir, err := exec.Run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotARepo
	}
	return dir, nil
}
