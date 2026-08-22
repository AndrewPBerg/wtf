package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/jj"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

// These reports deliberately keep durable identity and physical state in
// separate objects. A path or JJ name is not a substitute for a WTF UUID.
type workspaceReport struct {
	Identity      identity.Workspace `json:"identity"`
	Physical      physicalReport     `json:"physical"`
	JJ            *jjReport          `json:"jj,omitempty"`
	GitDiffShadow shadowReport       `json:"git_diff_shadow"`
}

type physicalReport struct {
	Present     bool   `json:"present"`
	Path        string `json:"path"`
	PathMatches bool   `json:"path_matches"`
	IsMain      bool   `json:"is_main"`
	Prunable    bool   `json:"prunable"`
	Branch      string `json:"branch"`
	Head        string `json:"head"`
	Error       string `json:"error,omitempty"`
}

type jjReport struct {
	Workspace string   `json:"workspace"`
	Change    string   `json:"change"`
	Commit    string   `json:"commit"`
	Bookmarks []string `json:"bookmarks,omitempty"`
	Operation string   `json:"operation,omitempty"`
}

type shadowReport struct {
	Supported bool   `json:"supported"`
	Status    string `json:"status"`
}

type workspaceListReport struct {
	Workspaces []workspaceReport `json:"workspaces"`
}

func init() {
	workspaceCmd.AddCommand(workspaceInspectCmd, workspaceListCmd)
	rootCmd.AddCommand(workspaceCmd)
}

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Inspect managed workspaces",
}

var workspaceInspectCmd = &cobra.Command{
	Use:   "inspect <workspace-id|name|path>",
	Short: "Inspect one managed workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := identity.DefaultStore()
		if err != nil {
			return err
		}
		workspace, err := store.LookupWorkspace(args[0])
		if err != nil {
			return err
		}
		report, err := inspectWorkspace(workspace)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), report)
		}
		printWorkspaceReport(cmd, report)
		return nil
	},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed workspaces",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := identity.DefaultStore()
		if err != nil {
			return err
		}
		state, err := store.Load()
		if err != nil {
			return err
		}
		reports := make([]workspaceReport, 0, len(state.Workspaces))
		for _, workspace := range state.Workspaces {
			report, reportErr := inspectWorkspace(workspace)
			if reportErr != nil {
				return reportErr
			}
			reports = append(reports, report)
		}
		sort.Slice(reports, func(i, j int) bool { return reports[i].Identity.ID < reports[j].Identity.ID })
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), workspaceListReport{Workspaces: reports})
		}
		for _, report := range reports {
			printWorkspaceReport(cmd, report)
		}
		return nil
	},
}

func inspectWorkspace(workspace identity.Workspace) (workspaceReport, error) {
	report := workspaceReport{
		Identity:      workspace,
		Physical:      physicalReport{Path: workspace.Path},
		GitDiffShadow: shadowReport{Supported: workspace.Backend == string(identity.JJ), Status: "not_supported"},
	}
	if workspace.Backend == string(identity.JJ) {
		report.GitDiffShadow.Status = "absent"
	}

	stateStore, err := identity.DefaultStore()
	if err != nil {
		return report, err
	}
	state, err := stateStore.Load()
	if err != nil {
		return report, err
	}
	var repo identity.Repository
	for _, candidate := range state.Repositories {
		if candidate.ID == workspace.RepositoryID {
			repo = candidate
			break
		}
	}
	if repo.ID == "" {
		return report, fmt.Errorf("repository %q for workspace %q not found", workspace.RepositoryID, workspace.ID)
	}

	kind, err := vcs.ParseKind(workspace.Backend)
	if err != nil {
		return report, err
	}
	if !vcs.Available(kind) {
		report.Physical.Error = fmt.Sprintf("%s is not available", kind.Label())
		return report, nil
	}
	mgr := newManager(kind)
	worktrees, err := mgr.List(repo.Locator)
	if err != nil {
		report.Physical.Error = err.Error()
		return report, nil
	}
	for _, wt := range worktrees {
		pathMatches := filepath.Clean(wt.Path) == filepath.Clean(workspace.Path)
		if !pathMatches && (!wt.Prunable || wt.NativeName != workspace.NativeName) {
			continue
		}
		report.Physical = physicalReport{Present: pathMatches && !wt.Prunable, Path: wt.Path, PathMatches: pathMatches, IsMain: wt.IsMain, Prunable: wt.Prunable, Branch: wt.Branch, Head: wt.Head}
		if !pathMatches {
			report.Physical.Error = "prunable VCS registration does not match the durable workspace path"
		}
		if kind == vcs.KindJJ {
			report.JJ = &jjReport{Workspace: wt.NativeName, Change: wt.ChangeID, Commit: wt.Head, Bookmarks: wt.Bookmarks}
			if shadow := vcs.IsJJGitDiffShadow(wt.Path); shadow {
				report.GitDiffShadow.Status = "present"
			}
			if op, ok := mgr.(*jj.WorkspaceManager); ok {
				if operation, opErr := op.CurrentOperationID(wt.Path); opErr == nil {
					report.JJ.Operation = operation
				}
			}
		}
		break
	}
	return report, nil
}

func printWorkspaceReport(cmd *cobra.Command, report workspaceReport) {
	i := report.Identity
	physical := "missing"
	if report.Physical.Present {
		physical = "present"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%s)\n", cyan(i.Name), dim(string(i.LifecycleState)), physical)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  id: %s\n  path: %s\n  backend: %s\n  native: %s\n", i.ID, i.Path, i.Backend, i.NativeName)
	if report.JJ != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  jj: workspace=%s change=%s commit=%s operation=%s\n", report.JJ.Workspace, report.JJ.Change, report.JJ.Commit, report.JJ.Operation)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  git-diff-shadow: %s\n", report.GitDiffShadow.Status)
	if report.Physical.Error != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  physical-error: %s\n", strings.TrimSpace(report.Physical.Error))
	}
}
