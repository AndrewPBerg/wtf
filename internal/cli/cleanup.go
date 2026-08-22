package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

// CleanupPlan is the deliberately small, serializable contract for destructive
// workspace cleanup. It contains a snapshot, not an instruction to discover a
// workspace again at apply time.
type CleanupPlan struct {
	Version       int                 `json:"version"`
	PlanID        string              `json:"plan_id"`
	Workspace     identity.Workspace  `json:"workspace"`
	Repository    identity.Repository `json:"repository"`
	Observed      *vcs.Worktree       `json:"observed,omitempty"`
	Preconditions []string            `json:"preconditions"`
	Actions       []string            `json:"actions"`
	Risks         []string            `json:"risks"`
}

var cleanupCmd = &cobra.Command{Use: "cleanup", Short: "Plan or apply managed workspace cleanup"}
var cleanupPlanCmd = &cobra.Command{
	Use:   "plan <workspace-id|name|path>",
	Short: "Create a read-only workspace cleanup plan",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runCleanupPlan(cmd, args[0]) },
}
var cleanupApplyCmd = &cobra.Command{
	Use:   "apply <plan-artifact>",
	Short: "Apply an explicit workspace cleanup plan artifact",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runCleanupApply(cmd, args[0]) },
}

func init() {
	cleanupCmd.AddCommand(cleanupPlanCmd, cleanupApplyCmd)
	rootCmd.AddCommand(cleanupCmd)
}

func planID(p CleanupPlan) string {
	p.PlanID = ""
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// cleanupWorktree resolves only the registered physical path. A path-less
// prunable registration may be repaired by its native VCS name because it has
// no physical checkout to confuse with another workspace.
func cleanupWorktree(worktrees []vcs.Worktree, workspace identity.Workspace) (vcs.Worktree, bool) {
	for _, worktree := range worktrees {
		canonical, err := identity.CanonicalPhysicalPath(worktree.Path)
		if err == nil && canonical == workspace.Path {
			return worktree, true
		}
		if worktree.Prunable && worktree.Path == "" && (worktree.NativeName == workspace.NativeName || worktree.Branch == workspace.NativeName) {
			return worktree, true
		}
	}
	return vcs.Worktree{}, false
}

func runCleanupPlan(cmd *cobra.Command, selector string) error {
	store, err := identity.DefaultStore()
	if err != nil {
		return err
	}
	workspace, err := store.LookupWorkspace(selector)
	if err != nil {
		return err
	}
	if workspace.LifecycleState != identity.Active {
		return fmt.Errorf("workspace %s is %s, not active", workspace.ID, workspace.LifecycleState)
	}
	repo, err := store.LookupRepository(workspace.RepositoryID)
	if err != nil {
		return fmt.Errorf("loading workspace repository: %w", err)
	}
	if _, err := os.Stat(workspace.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("checking workspace path: %w", err)
	}
	kind, err := vcs.ParseKind(workspace.Backend)
	if err != nil {
		return err
	}
	wm := newManager(kind)
	wts, err := wm.List(repo.Locator)
	if err != nil {
		return fmt.Errorf("checking workspace registration: %w", err)
	}
	var observed *vcs.Worktree
	if worktree, ok := cleanupWorktree(wts, workspace); ok {
		observed = &worktree
	}
	if observed == nil && !pathMissing(workspace.Path) {
		return fmt.Errorf("workspace %s is not registered by %s", workspace.ID, kind.Label())
	}

	p := CleanupPlan{
		Version: 1, Workspace: workspace, Repository: repo, Observed: observed,
		Preconditions: []string{
			"workspace identity is active and unchanged",
			"repository identity is active and unchanged",
			"VCS registration and physical path match this snapshot",
			"workspace is not the main checkout or the current directory",
		},
		Actions: []string{
			"stop the workspace dev server if one is recorded",
			"remove the VCS workspace and its physical directory",
			"release the workspace port allocation",
			"record the workspace identity tombstone",
		},
		Risks: []string{
			"uncommitted changes can prevent removal unless the existing rm force policy is used",
			"the workspace directory, env links/copies, logs, and editor shadow metadata under it are removed with the directory",
		},
	}
	p.PlanID = planID(p)
	// Plan output is always JSON so it can be redirected directly to apply.
	return writeJSON(cmd.OutOrStdout(), p)
}

func readCleanupPlan(path string) (CleanupPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CleanupPlan{}, fmt.Errorf("reading cleanup plan: %w", err)
	}
	var p CleanupPlan
	if err := json.Unmarshal(data, &p); err != nil {
		return CleanupPlan{}, fmt.Errorf("parsing cleanup plan: %w", err)
	}
	if p.Version != 1 || p.PlanID == "" || p.PlanID != planID(p) {
		return CleanupPlan{}, fmt.Errorf("invalid or altered cleanup plan")
	}
	return p, nil
}

func runCleanupApply(cmd *cobra.Command, artifact string) error {
	p, err := readCleanupPlan(artifact)
	if err != nil {
		return err
	}
	store, err := identity.DefaultStore()
	if err != nil {
		return err
	}
	workspace, err := store.LookupWorkspace(p.Workspace.ID)
	if err != nil {
		return fmt.Errorf("checking cleanup preconditions: %w", err)
	}
	repo, err := store.LookupRepository(p.Repository.ID)
	if err != nil || !reflect.DeepEqual(repo, p.Repository) {
		return fmt.Errorf("cleanup plan %s is stale: repository identity changed", p.PlanID)
	}
	if repo.LifecycleState != identity.Active {
		return fmt.Errorf("cleanup plan %s is stale: repository is no longer active", p.PlanID)
	}
	if workspace.LifecycleState != identity.Removed && !reflect.DeepEqual(workspace, p.Workspace) {
		return fmt.Errorf("cleanup plan %s is stale: workspace identity changed", p.PlanID)
	}
	// A valid plan whose identity is now a removed tombstone was already applied.
	// Treat this as an idempotent success, but only when immutable identity fields
	// still match the plan; altered plans were rejected above by readCleanupPlan.
	if workspace.LifecycleState == identity.Removed {
		if workspace.RepositoryID != p.Workspace.RepositoryID || workspace.Name != p.Workspace.Name || workspace.Backend != p.Workspace.Backend || workspace.NativeName != p.Workspace.NativeName || workspace.Path != p.Workspace.Path {
			return fmt.Errorf("cleanup plan %s is stale: removed workspace identity changed", p.PlanID)
		}
		if jsonOutput {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"version": 1, "plan_id": p.PlanID, "workspace_id": p.Workspace.ID, "applied": true, "noop": true})
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Workspace %s was already removed\n", p.Workspace.ID)
		return err
	}

	kind, err := vcs.ParseKind(workspace.Backend)
	if err != nil {
		return err
	}
	wm := newManager(kind)
	wts, err := wm.List(repo.Locator)
	if err != nil {
		return fmt.Errorf("checking cleanup preconditions: %w", err)
	}
	target, found := cleanupWorktree(wts, workspace)
	if p.Observed != nil {
		pathsMatch := target.Path == "" && p.Observed.Path == ""
		if !pathsMatch {
			targetPath, targetPathErr := identity.CanonicalPhysicalPath(target.Path)
			observedPath, observedPathErr := identity.CanonicalPhysicalPath(p.Observed.Path)
			pathsMatch = targetPathErr == nil && observedPathErr == nil && targetPath == observedPath
		}
		if !found || !pathsMatch || target.IsMain != p.Observed.IsMain || target.Branch != p.Observed.Branch || target.NativeName != p.Observed.NativeName || target.Head != p.Observed.Head || target.Prunable != p.Observed.Prunable {
			return fmt.Errorf("cleanup plan %s is stale: VCS workspace registration changed", p.PlanID)
		}
	}
	if target.Path == "" && !pathMissing(workspace.Path) {
		return fmt.Errorf("cleanup plan %s is stale: VCS workspace registration changed", p.PlanID)
	}
	if target.Path == "" {
		target = vcs.Worktree{Path: workspace.Path, Branch: workspace.NativeName, NativeName: workspace.NativeName, WorkspaceID: workspace.ID, RepositoryID: workspace.RepositoryID, VCS: kind}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if target.IsMain || vcs.IsInside(cwd, target.Path) {
		return fmt.Errorf("cleanup plan %s cannot be applied from the target workspace or main checkout", p.PlanID)
	}
	target.WorkspaceID, target.RepositoryID = workspace.ID, workspace.RepositoryID
	oldForce := rmForce
	rmForce = false // plans never silently broaden rm's force policy
	defer func() { rmForce = oldForce }()
	if err := removePhysicalAndIdentity(cmd, wm, repo.Locator, nativeWorktreeRef(target), cwd, target); err != nil {
		return fmt.Errorf("applying cleanup plan %s: %w", p.PlanID, err)
	}
	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), map[string]any{"version": 1, "plan_id": p.PlanID, "applied": true, "noop": false, "workspace_id": p.Workspace.ID})
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed workspace %s\n", p.Workspace.ID)
	return err
}

func pathMissing(path string) bool { _, err := os.Lstat(path); return os.IsNotExist(err) }
