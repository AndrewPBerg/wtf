package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/spf13/cobra"
)

var registerList bool

func init() {
	registerCmd.Flags().BoolVarP(&registerList, "list", "l", false, "List all registered repos after registering")
	rootCmd.AddCommand(registerCmd)
}

var registerCmd = &cobra.Command{
	Use:   "register [path...]",
	Short: "Register a local repo in the wtf registry",
	Long: `Register one or more local repos in the wtf registry (~/.wtf/repos.json).
If no path is given, registers the current repo. Accepts absolute or relative paths.

Examples:
  wtf register                    # register the current repo
  wtf register ../my-app          # register a relative path
  wtf register /home/user/my-lib  # register an absolute path
  wtf register . ../other-repo    # register multiple repos
  wtf register -l                 # register current repo and list all`,
	Args: cobra.ArbitraryArgs,
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var paths []string

		if len(args) == 0 {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			dir, err := getRepoDirWithoutRegister(cwd)
			if err != nil {
				return err
			}
			paths = append(paths, dir)
		} else {
			for _, arg := range args {
				resolved, err := resolveAndValidateRepo(arg)
				if err != nil {
					return err
				}
				paths = append(paths, resolved)
			}
		}

		var registered []string
		for _, p := range paths {
			if err := config.Add(p); err != nil {
				return fmt.Errorf("registering repo: %w", err)
			}
			registered = append(registered, p)
		}

		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{
				"registered": registered,
			})
		}

		for _, p := range registered {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Registered %s\n", greenBold("✔"), cyan(p))
		}

		if registerList {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return runRepos(cmd)
		}

		return nil
	},
}

// resolveAndValidateRepo resolves a path argument to an absolute path and
// validates that it is a git repository.
func resolveAndValidateRepo(arg string) (string, error) {
	var abs string
	if filepath.IsAbs(arg) {
		abs = arg
	} else {
		resolved, err := filepath.Abs(arg)
		if err != nil {
			return "", fmt.Errorf("resolving path %s: %w", arg, err)
		}
		abs = resolved
	}

	// Check it's a git repo
	gitDir := filepath.Join(abs, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a git repository", abs)
	}

	return abs, nil
}
