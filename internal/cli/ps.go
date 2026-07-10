package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/AndrewPBerg/wtf/internal/workspace"
	"github.com/spf13/cobra"
)

var psGlobal bool

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.Flags().BoolVarP(&psGlobal, "global", "g", false, "List instances from registered workspaces")
}

var psCmd = &cobra.Command{Use: "ps", Short: "List running profile instances in this workspace", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return runPS(cmd) }}

func runPS(cmd *cobra.Command) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	var roots []string
	if psGlobal {
		roots, err = workspace.Registered()
		if err != nil {
			return fmt.Errorf("listing registered workspaces: %w", err)
		}
	} else {
		_, root, loadErr := workspace.Load(dir)
		if loadErr != nil {
			return loadErr
		}
		if err := workspace.Register(root); err != nil {
			return fmt.Errorf("registering workspace: %w", err)
		}
		roots = []string{root}
	}
	states := []workspace.RuntimeState{}
	for _, workspaceRoot := range roots {
		listed, listErr := workspace.List(workspaceRoot)
		if listErr != nil {
			return fmt.Errorf("listing instances in %s: %w", workspaceRoot, listErr)
		}
		states = append(states, listed...)
	}
	if err != nil {
		return fmt.Errorf("listing instances: %w", err)
	}
	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), states)
	}
	if len(states) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No profile instances")
		return err
	}
	for _, state := range states {
		ports := []int{}
		for _, service := range state.Services {
			for _, port := range service.Ports {
				ports = append(ports, port)
			}
		}
		sort.Ints(ports)
		status := "stopped"
		if workspace.IsRunning(state) {
			status = "running"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  ports %v  %s\n", state.Instance, state.Profile, ports, status)
	}
	return nil
}
