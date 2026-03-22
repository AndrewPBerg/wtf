package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage project configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a default .wt-forge.toml for this repo",
	Long: `Generate a .wt-forge.toml configuration file with sensible defaults
based on the current repository.

Detects the default branch, env files, and package manager automatically.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		exec := &git.RealExecutor{}
		wm := git.NewWorktreeManager(exec)
		bm := git.NewBranchManager(exec)
		return runConfigInit(cmd, wm, bm)
	},
}

func runConfigInit(cmd *cobra.Command, wm *git.WorktreeManager, bm *git.BranchManager) error {
	dir, err := getRepoDir()
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, config.ProjectConfigFile)
	if _, err := os.Stat(dest); err == nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s already exists. Overwrite? [y/N] ", cyan(config.ProjectConfigFile))

		reader := bufio.NewReader(cmd.InOrStdin())
		answer, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("reading input: %w", readErr)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), yellow("Aborted."))
			return nil
		}
	}

	// Detect default branch from main worktree
	mainWt, err := wm.MainWorktree(dir)
	if err != nil {
		return fmt.Errorf("finding main worktree: %w", err)
	}

	defaultBase := mainWt.Branch
	if defaultBase == "" {
		// Fallback: use current branch
		defaultBase, _ = bm.CurrentBranch(dir)
	}
	if defaultBase == "" {
		defaultBase = "main"
	}

	// Detect env files that exist in the repo
	var envFiles []string
	for _, f := range setup.DefaultEnvFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			envFiles = append(envFiles, f)
		}
	}

	// Detect package manager
	var installCmd string
	if pm, _ := setup.DetectPackageManager(dir); pm != nil {
		installCmd = pm.InstallCmd
	}

	content := config.GenerateDefaultConfig(config.DefaultConfigOptions{
		DefaultBase: defaultBase,
		EnvFiles:    envFiles,
		InstallCmd:  installCmd,
	})

	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Created %s\n", greenBold("✔"), cyan(config.ProjectConfigFile))

	// Ensure .wt-forge.toml is in .gitignore
	added, gitignoreErr := config.EnsureGitignore(dir)
	if gitignoreErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s could not update .gitignore: %v\n", yellow("⚠"), gitignoreErr)
	} else if added {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Added %s to .gitignore\n", greenBold("✔"), cyan(config.ProjectConfigFile))
	}

	return nil
}
