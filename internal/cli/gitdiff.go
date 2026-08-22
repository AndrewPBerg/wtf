package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(gitDiffCmd)
}

var gitDiffCmd = &cobra.Command{
	Use:   "git-diff",
	Short: "Create or refresh Git metadata for a jj workspace editor diff",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		wm, err := resolveManager(cmd)
		if err != nil {
			return err
		}
		if wm.Kind() != vcs.KindJJ {
			return fmt.Errorf("git-diff is only available for jj workspaces")
		}
		manager, ok := wm.(vcs.GitDiffManager)
		if !ok {
			return fmt.Errorf("active jj backend does not support Git diff metadata")
		}
		workspacePath, err := repoDirFor(wm)
		if err != nil {
			return err
		}

		marker := filepath.Join(workspacePath, ".git", vcs.JJGitDiffMarker)
		action := "Refreshed"
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			action = "Created"
			err = manager.InitGitDiff(workspacePath)
			if err != nil {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("checking Git diff metadata: %w", err)
		} else if err := manager.RefreshGitDiff(workspacePath); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s Git diff metadata at %s\n",
			greenBold("✔"), action, cyan(workspacePath))
		return nil
	},
}
