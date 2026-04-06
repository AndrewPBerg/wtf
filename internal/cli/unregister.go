package cli

import (
	"fmt"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(unregisterCmd)
}

var unregisterCmd = &cobra.Command{
	Use:   "unregister [name|path]",
	Short: "Remove a repo from the wtf registry",
	Long: `Remove a repo from the wtf registry (~/.wtf/repos.json).
If no argument is given, opens an interactive picker to select repos to unregister.
Accepts a repo name or path.

Examples:
  wtf unregister                # pick repos to unregister interactively
  wtf unregister .              # unregister the current repo
  wtf unregister /path/to/repo  # unregister by path
  wtf unregister my-app         # unregister by name`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRegisteredRepos,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return unregisterSingle(cmd, args[0])
		}

		// No args: show picker with all registered repos.
		paths, err := config.LoadValid()
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}
		if len(paths) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("No registered repos."))
			return nil
		}

		items := make([]ui.RepoPickerItem, len(paths))
		for i, p := range paths {
			items[i] = ui.RepoPickerItem{
				Name:       filepath.Base(p),
				Path:       p,
				Registered: true,
			}
		}

		result, err := ui.RunRepoPicker(items, ui.RepoPickerUnregister)
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

		var removed []string
		for _, item := range result.Items {
			ok, err := config.Remove(item.Path)
			if err != nil {
				return fmt.Errorf("unregistering repo: %w", err)
			}
			if ok {
				removed = append(removed, item.Path)
			}
		}

		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{
				"unregistered": removed,
			})
		}

		for _, p := range removed {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Unregistered %s\n", greenBold("✔"), cyan(p))
		}
		return nil
	},
}

// unregisterSingle removes a single repo given as a CLI argument.
func unregisterSingle(cmd *cobra.Command, arg string) error {
	repoPath := resolveRepoArg(arg)

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
