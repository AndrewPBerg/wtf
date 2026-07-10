package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/workspace"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(planCmd)
}

var planCmd = &cobra.Command{
	Use:   "plan <profile> <instance>",
	Short: "Show the environment a profile would materialize",
	Long: `Resolve a profile from .wtf/workspace.yaml without creating worktrees,
copying environment files, allocating ports, or starting processes.

This is the first, deliberately side-effect-free step of WTF's profile-driven
workspace spike.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlan(cmd, args[0], args[1])
	},
}

func runPlan(cmd *cobra.Command, profile, instance string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	config, root, err := workspace.Load(dir)
	if err != nil {
		return err
	}
	plan, err := workspace.BuildPlan(config, root, profile, instance)
	if err != nil {
		return err
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), plan)
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Profile %s · instance %s\n", cyan(plan.Profile), cyan(plan.Instance))
	_, _ = fmt.Fprintf(out, "workspace: %s\n", plan.Workspace)
	for _, service := range plan.Services {
		_, _ = fmt.Fprintf(out, "\n%s\n  dir: %s\n  source: %s\n", bold(service.Name), service.Dir, service.Source)
		if service.WorktreeFor != "" {
			_, _ = fmt.Fprintf(out, "  worktree: %s\n", service.WorktreeFor)
		}
		if env := service.Env; env != nil {
			_, _ = fmt.Fprintf(out, "  env: %s (%s)\n", env.Path, env.Mode)
		}
		portNames := make([]string, 0, len(service.Ports))
		for name := range service.Ports {
			portNames = append(portNames, name)
		}
		sort.Strings(portNames)
		for _, name := range portNames {
			port := service.Ports[name]
			_, _ = fmt.Fprintf(out, "  port: %s (%d-%d)\n", name, port.From, port.To)
		}
		if service.Up != "" {
			_, _ = fmt.Fprintf(out, "  up: %s\n", strings.TrimSpace(service.Up))
		}
	}
	return nil
}
