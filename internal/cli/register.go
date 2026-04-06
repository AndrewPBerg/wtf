package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/ui"
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
If no path is given, discovers git repos in the current directory and opens
an interactive picker. Accepts absolute or relative paths.

Examples:
  wtf register                    # discover repos and pick interactively
  wtf register ../my-app          # register a relative path
  wtf register /home/user/my-lib  # register an absolute path
  wtf register . ../other-repo    # register multiple repos
  wtf register -l                 # register and list all`,
	Args: cobra.ArbitraryArgs,
	ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Explicit paths: register directly, no picker.
		if len(args) > 0 {
			return registerPaths(cmd, args)
		}

		// No args: discover repos and show picker.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		items, err := discoverRepos(cwd)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			return fmt.Errorf("no git repos found in %s", cwd)
		}

		// Check if all discovered repos are already registered.
		allRegistered := true
		for _, item := range items {
			if !item.Registered {
				allRegistered = false
				break
			}
		}
		if allRegistered {
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"registered": []string{}})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("All discovered repos are already registered."))
			return nil
		}

		result, err := ui.RunRepoPicker(items, ui.RepoPickerRegister)
		if err != nil {
			return err
		}
		if result.Quit {
			return nil
		}
		if len(result.Items) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("Nothing selected."))
			return nil
		}

		var paths []string
		for _, item := range result.Items {
			paths = append(paths, item.Path)
		}

		return registerAndReport(cmd, paths)
	},
}

// registerPaths registers explicitly provided path arguments.
func registerPaths(cmd *cobra.Command, args []string) error {
	var paths []string
	for _, arg := range args {
		resolved, err := resolveAndValidateRepo(arg)
		if err != nil {
			return err
		}
		paths = append(paths, resolved)
	}
	return registerAndReport(cmd, paths)
}

// registerAndReport registers the given paths and prints results.
func registerAndReport(cmd *cobra.Command, paths []string) error {
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
}

// discoverRepos scans the directory for git repos (itself + immediate children).
func discoverRepos(dir string) ([]ui.RepoPickerItem, error) {
	registered, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	regSet := make(map[string]bool, len(registered))
	for _, r := range registered {
		regSet[r] = true
	}

	var items []ui.RepoPickerItem

	// Check if the current directory itself is a repo.
	if isGitRepo(dir) {
		items = append(items, ui.RepoPickerItem{
			Name:       filepath.Base(dir),
			Path:       dir,
			Registered: regSet[dir],
		})
	}

	// Scan immediate children.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return items, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		childPath := filepath.Join(dir, entry.Name())
		if isGitRepo(childPath) {
			items = append(items, ui.RepoPickerItem{
				Name:       entry.Name(),
				Path:       childPath,
				Registered: regSet[childPath],
			})
		}
	}

	return items, nil
}

// isGitRepo checks if a directory contains a .git directory.
func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
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

	// Check it's a git repo.
	gitDir := filepath.Join(abs, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a git repository", abs)
	}

	return abs, nil
}
