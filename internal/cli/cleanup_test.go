package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupWorktreeMatchesOnlyRecordedPathOrPathlessPrunableRegistration(t *testing.T) {
	workspace := identity.Workspace{Path: "/repo/target", NativeName: "target"}
	worktrees := []vcs.Worktree{
		{Path: "/repo/other", NativeName: "target", Branch: "target"},
		{Path: "/repo/target", NativeName: "different", Branch: "different"},
	}
	found, ok := cleanupWorktree(worktrees, workspace)
	require.True(t, ok)
	require.Equal(t, "/repo/target", found.Path)

	worktrees = []vcs.Worktree{{Path: "", NativeName: "target", Prunable: true}}
	found, ok = cleanupWorktree(worktrees, workspace)
	require.True(t, ok)
	require.Empty(t, found.Path)

	_, ok = cleanupWorktree([]vcs.Worktree{{Path: "/repo/other", NativeName: "target", Branch: "target"}}, workspace)
	require.False(t, ok)
}

func TestCleanupPlanIsIdentityBoundAndReadOnly(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	path, err := wm.Add(repo, "cleanup-target", "main")
	require.NoError(t, err)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	r, err := store.CreateRepository(repo)
	require.NoError(t, err)
	w, err := store.CreateWorkspace(r.ID, "repo/cleanup-target", "git", "cleanup-target", path)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(w.ID)
	require.NoError(t, err)

	out := new(bytes.Buffer)
	cmd := cleanupPlanCmd
	cmd.SetOut(out)
	require.NoError(t, runCleanupPlan(cmd, w.ID))
	var plan CleanupPlan
	require.NoError(t, json.Unmarshal(out.Bytes(), &plan))
	require.Equal(t, w.ID, plan.Workspace.ID)
	require.NotEmpty(t, plan.PlanID)
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestCleanupPlanIsRepeatableAndApplyIsIdempotent(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	path, err := wm.Add(repo, "cleanup-repeat", "main")
	require.NoError(t, err)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	r, err := store.CreateRepository(repo)
	require.NoError(t, err)
	w, err := store.CreateWorkspace(r.ID, "repo/cleanup-repeat", "git", "cleanup-repeat", path)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(w.ID)
	require.NoError(t, err)

	first, second := new(bytes.Buffer), new(bytes.Buffer)
	cleanupPlanCmd.SetOut(first)
	require.NoError(t, runCleanupPlan(cleanupPlanCmd, w.ID))
	cleanupPlanCmd.SetOut(second)
	require.NoError(t, runCleanupPlan(cleanupPlanCmd, "repo/cleanup-repeat"))
	assert.Equal(t, first.String(), second.String())
	planFile := filepath.Join(t.TempDir(), "cleanup.json")
	require.NoError(t, os.WriteFile(planFile, first.Bytes(), 0o600))

	oldJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = oldJSON })
	applyOut := new(bytes.Buffer)
	cleanupApplyCmd.SetOut(applyOut)
	require.NoError(t, runCleanupApply(cleanupApplyCmd, planFile))
	applyOut.Reset()
	require.NoError(t, runCleanupApply(cleanupApplyCmd, planFile))
	var result map[string]any
	require.NoError(t, json.Unmarshal(applyOut.Bytes(), &result))
	assert.Equal(t, true, result["noop"])
}

func TestCleanupFailedUUIDRetryAndRemovedJSONNoop(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	path, err := wm.Add(repo, "cleanup-retry", "main")
	require.NoError(t, err)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	r, err := store.CreateRepository(repo)
	require.NoError(t, err)
	w, err := store.CreateWorkspace(r.ID, "repo/cleanup-retry", "git", "cleanup-retry", path)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(w.ID)
	require.NoError(t, err)
	require.NoError(t, wm.Remove(repo, "cleanup-retry", repo, true))
	_, err = store.MarkCleanupFailed(w.ID)
	require.NoError(t, err)

	oldJSON := jsonOutput
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = oldJSON })
	cmd := rmCmd
	cmd.SetOut(new(bytes.Buffer))
	require.NoError(t, runRm(cmd, w.ID, wm))
	removed, err := store.LookupWorkspace(w.ID)
	require.NoError(t, err)
	assert.Equal(t, identity.Removed, removed.LifecycleState)
	// A second UUID retry is a structured no-op and does not require VCS state.
	require.NoError(t, runRm(cmd, w.ID, wm))
}

func TestCleanupApplyFailsClosedWhenIdentityChanges(t *testing.T) {
	repo := initCLITestRepo(t)
	t.Chdir(repo)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	path, err := wm.Add(repo, "cleanup-stale", "main")
	require.NoError(t, err)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	r, err := store.CreateRepository(repo)
	require.NoError(t, err)
	w, err := store.CreateWorkspace(r.ID, "repo/cleanup-stale", "git", "cleanup-stale", path)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(w.ID)
	require.NoError(t, err)

	planFile := filepath.Join(t.TempDir(), "cleanup.json")
	planOut := new(bytes.Buffer)
	cmd := cleanupPlanCmd
	cmd.SetOut(planOut)
	require.NoError(t, runCleanupPlan(cmd, w.ID))
	require.NoError(t, os.WriteFile(planFile, planOut.Bytes(), 0o600))
	_, err = store.RenameWorkspace(w.ID, "repo/renamed")
	require.NoError(t, err)

	applyOut := new(bytes.Buffer)
	applyCmd := cleanupApplyCmd
	applyCmd.SetOut(applyOut)
	err = runCleanupApply(applyCmd, planFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "identity changed")
	_, err = os.Stat(path)
	require.NoError(t, err)
}
