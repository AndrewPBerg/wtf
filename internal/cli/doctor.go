package cli

import (
	"fmt"
	"sort"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/spf13/cobra"
)

type doctorReport struct {
	Version  int       `json:"version"`
	Findings []Finding `json:"findings"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [workspace-id|name|path]",
	Short: "Diagnose managed workspaces without changing them",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := identity.DefaultStore()
		if err != nil {
			return err
		}
		state, err := store.Load()
		if err != nil {
			return err
		}
		workspaces, err := selectedWorkspaces(state, args)
		if err != nil {
			return err
		}
		findings, err := doctorWorkspaces(workspaces)
		if err != nil {
			return err
		}
		report := doctorReport{Version: ReportVersion, Findings: findings}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), report)
		}
		for _, f := range findings {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", f.Code, f.Message)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(doctorCmd) }

func doctorWorkspaces(workspaces []identity.Workspace) ([]Finding, error) {
	// Resource collection intentionally performs only metadata reads and is also
	// the single source for file/port checks, keeping doctor and resources alike.
	ids := make([]string, 0, len(workspaces))
	byID := make(map[string]identity.Workspace, len(workspaces))
	for _, w := range workspaces {
		ids = append(ids, w.ID)
		byID[w.ID] = w
	}
	sort.Strings(ids)
	store, err := identity.DefaultStore()
	if err != nil {
		return nil, err
	}
	// Collect a complete report, then retain only the requested UUIDs for
	// deterministic joins without re-reading or mutating identity state.
	all, findings, err := collectResourceReports(store, nil)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	filtered := findings[:0]
	for _, f := range findings {
		if allowed[f.WorkspaceID] {
			filtered = append(filtered, f)
		}
	}
	findings = filtered
	for _, id := range ids {
		w := byID[id]
		if w.LifecycleState == identity.CleanupFailed {
			findings = append(findings, finding("cleanup_failed", SeverityError, w.RepositoryID, w.ID, "workspace cleanup is incomplete"))
		}
		item, ok := all[id]
		if !ok {
			continue
		}
		if item.Lifecycle == "cleanup_failed" && len(item.CleanupDebt) == 0 {
			findings = append(findings, finding("resource_cleanup_debt", SeverityWarning, w.RepositoryID, w.ID, "resource cleanup debt remains recorded"))
		}
		for _, debt := range item.CleanupDebt {
			findings = append(findings, finding("resource_cleanup_debt", SeverityWarning, w.RepositoryID, w.ID, fmt.Sprintf("%s %s cleanup debt: %s", debt.Kind, debt.Name, debt.Reason)))
		}
		// Resource collection already performs the authoritative VCS registration
		// observation. Avoid inspecting the workspace again: duplicate backend
		// reads previously produced duplicate diagnoses.
		if w.Backend == string(identity.JJ) {
			shadow := inspectGitDiffShadow(w.Path, "")
			switch shadow.Status {
			case "absent":
				findings = append(findings, finding("git_shadow_missing", SeverityWarning, w.RepositoryID, w.ID, "managed Git shadow is missing"))
			case "stale":
				findings = append(findings, finding("git_shadow_stale", SeverityWarning, w.RepositoryID, w.ID, "managed Git shadow is stale"))
			case "unavailable":
				findings = append(findings, finding("git_shadow_unavailable", SeverityError, w.RepositoryID, w.ID, shadow.Error))
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RepositoryID != findings[j].RepositoryID {
			return findings[i].RepositoryID < findings[j].RepositoryID
		}
		if findings[i].WorkspaceID != findings[j].WorkspaceID {
			return findings[i].WorkspaceID < findings[j].WorkspaceID
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Message < findings[j].Message
	})
	return findings, nil
}
