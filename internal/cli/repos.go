package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(reposCmd)
}

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List all registered repos",
	Long: `List all repos registered in the wtf registry (~/.wtf/repos.json).

Repos are auto-registered when you run wtf commands inside them.
Use 'wtf unregister [path]' to remove a repo from the registry.

Examples:
  wtf repos              # list all registered repos
  wtf unregister .       # unregister current repo
  wtf unregister /path   # unregister by path`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runRepos(cmd)
	},
}

func runRepos(cmd *cobra.Command) error {
	// Auto-prune stale entries (temp dirs from tests, deleted repos).
	paths, err := config.Prune()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if len(paths) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), dim("No registered repos. Run a wtf command inside a repo to auto-register it."))
		return nil
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), paths)
	}

	out := cmd.OutOrStdout()

	// Detect current repo to highlight it.
	cwd, _ := os.Getwd()
	currentRepo, _ := getRepoDirWithoutRegister(cwd)

	for _, p := range paths {
		name := filepath.Base(p)
		marker := "  "
		if p == currentRepo {
			marker = cyan("→ ")
			name = cyanBold(name)
		} else {
			name = bold(name)
		}

		_, _ = fmt.Fprintf(out, "%s%s %s\n", marker, name, dim(p))
	}

	_, _ = fmt.Fprintf(out, "\n%s\n", dim(fmt.Sprintf("%d repo(s) registered", len(paths))))
	return nil
}
