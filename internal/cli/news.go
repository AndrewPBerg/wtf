package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

var newsBase string

func init() {
	newsCmd.Flags().StringVar(&newsBase, "base", "main", "Base branch to create from")
	rootCmd.AddCommand(newsCmd)
}

var newsCmd = &cobra.Command{
	Use:   "news <branch>",
	Short: "Create a new worktree and switch to it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNews(cmd, args[0], git.NewWorktreeManager(&git.RealExecutor{}), setup.NewRunner())
	},
}

func runNews(cmd *cobra.Command, branch string, wm *git.WorktreeManager, runner *setup.Runner) error {
	bm := git.NewBranchManager(&git.RealExecutor{})
	if err := bm.ValidateBranchName(branch); err != nil {
		return err
	}

	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	wtPath, err := wm.Add(dir, branch, newsBase)
	if err != nil {
		return err
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]string{
			"path":   wtPath,
			"branch": branch,
		})
	}

	// Print path to stdout for the shell function to cd
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), wtPath)
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Created worktree at %s\n", greenBold("✔"), cyan(wtPath))

	// Run setup — failures are warnings, not errors
	if runner != nil {
		mainWt, mainErr := wm.MainWorktree(dir)
		if mainErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), mainErr)
			return nil
		}

		cfg, cfgErr := config.LoadProjectConfig(mainWt.Path)
		if cfgErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), cfgErr)
			return nil
		}

		if cfg != nil {
			if valErr := config.ValidateProjectConfig(cfg); valErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), valErr)
				return nil
			}
		}

		if setupErr := runner.RunSetup(cfg, mainWt.Path, wtPath, branch); setupErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup failed: %v\n", yellow("⚠"), setupErr)
		}
	}

	return nil
}
