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
	Use:               "unregister [name|path]",
	Short:             "Remove a repo from the wtf registry",
	Long:              `Remove a repo from the wtf registry (~/.wtf/repos.json). Accepts a repo name or path. If no argument is given, unregisters the current repo.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRegisteredRepos,
	RunE: func(cmd *cobra.Command, args []string) error {
		var repoPath string

		if len(args) == 1 {
			repoPath = resolveRepoArg(args[0])
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

// resolveRepoArg resolves a repo argument that can be a full path, relative path,
// or just a repo name (basename match against registered repos).
func resolveRepoArg(arg string) string {
	// If it looks like a path (contains separator or starts with . or /), resolve as path.
	if filepath.IsAbs(arg) || arg == "." || arg == ".." ||
		filepath.Dir(arg) != "." {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return arg
		}
		return abs
	}

	// Try matching by basename against registered repos.
	paths, err := config.LoadValid()
	if err == nil {
		for _, p := range paths {
			if filepath.Base(p) == arg {
				return p
			}
		}
	}

	// Fall back to treating as a relative path.
	abs, err := filepath.Abs(arg)
	if err != nil {
		return arg
	}
	return abs
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
