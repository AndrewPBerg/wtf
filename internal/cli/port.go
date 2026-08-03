package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(portCmd)
}

var portCmd = &cobra.Command{
	Use:   "port",
	Short: "Show the allocated port for the current worktree",
	Long: `Print the dev-server port assigned to the current worktree.

The base port is auto-detected from your project framework:
  Next.js / Nuxt / Remix  → 3000
  Astro                   → 4321
  Vite / SvelteKit        → 5173
  Angular                 → 4200
  Django                  → 8000
  Go                      → 8080

Each worktree gets a unique port (base+N). Use $PORT in your
dev server config to avoid conflicts across worktrees.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runPort(cmd)
	},
}

func runPort(cmd *cobra.Command) error {
	mgr, err := resolveManager(cmd)
	if err != nil {
		return err
	}

	dir, err := repoDirFor(mgr)
	if err != nil {
		return err
	}

	// git reports the current branch; jj reports the workspace name.
	branch, err := mgr.CurrentRef(dir)
	if err != nil {
		return err
	}

	alloc, err := portAllocator(mgr, dir)
	if err != nil {
		return err
	}

	p, err := alloc.Allocate(branch)
	if err != nil {
		return fmt.Errorf("allocating port: %w", err)
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"port":   p,
			"branch": branch,
		})
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), strconv.Itoa(p))
	return err
}
