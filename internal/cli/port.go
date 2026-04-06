package cli

import (
	"fmt"
	"strconv"

	"github.com/AndrewPBerg/wtf/internal/git"
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
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	exec := &git.RealExecutor{}
	branch, err := exec.Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("detecting current branch: %w", err)
	}

	alloc, err := portAllocator(dir)
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
