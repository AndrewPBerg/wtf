package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type cleanupCoverageManager struct {
	vcs.Manager
	state string
	err   error
}

func (m cleanupCoverageManager) StateDir(string) (string, error) { return m.state, m.err }

func TestCleanupResourcesMissingAndRegistryErrors(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	missing := cleanupCoverageManager{state: t.TempDir()}
	called := false
	require.NoError(t, cleanupResources(id, "repo", "workspace", missing, func() error { called = true; return nil }))
	require.False(t, called)

	stateErr := errors.New("state unavailable")
	require.ErrorIs(t, cleanupResources(id, "repo", "workspace", cleanupCoverageManager{err: stateErr}, nil), stateErr)

	state := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(state, "resources.json"), []byte("broken"), 0o600))
	require.ErrorContains(t, cleanupResources(id, "repo", "workspace", cleanupCoverageManager{state: state}, nil), "loading resource ownership")
}

func TestCleanupResourcesReleasesPortLeases(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	state := t.TempDir()
	path := filepath.Join(state, "resources.json")
	data := map[string]any{"version": 1, "workspaces": map[string]any{id: resource.Workspace{
		Version: 1, WorkspaceID: id, Lifecycle: resource.LifecycleActive,
		Desired: resource.Desired{Ports: []resource.PortIntent{{Name: "web", Preferred: 3000}}},
		Leases:  []resource.Lease{{Kind: resource.KindPort, Name: "web", ID: "lease-present", State: resource.LeaseAcquired}},
	}}}
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
	failed := false
	err = cleanupResources(id, "repo", "workspace", cleanupCoverageManager{state: state}, func() error { failed = true; return nil })
	require.NoError(t, err)
	require.False(t, failed)
	reg := resource.NewRegistry(path)
	ws, getErr := reg.Get(id)
	require.NoError(t, getErr)
	require.Equal(t, resource.LeaseReleased, ws.Leases[0].State)
}

func TestRunCleanupApplyRejectsMalformedAndChangedPlans(t *testing.T) {
	repoDir, workspacePath := initCLITestRepo(t), filepath.Join(t.TempDir(), "workspace")
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repo, err := store.CreateRepository(repoDir)
	require.NoError(t, err)
	workspace, err := store.CreateWorkspace(repo.ID, "repo/changed", "git", "changed", workspacePath)
	require.NoError(t, err)
	workspace, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)
	plan := CleanupPlan{Version: 1, Workspace: workspace, Repository: repo}
	plan.PlanID = planID(plan)
	artifact := filepath.Join(t.TempDir(), "plan.json")
	changed := plan
	changed.Repository.Locator = filepath.Join(t.TempDir(), "different")
	changed.PlanID = planID(changed)
	data, err := json.Marshal(changed)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(artifact, data, 0o600))
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	require.ErrorContains(t, runCleanupApply(cmd, artifact), "repository identity changed")

	bad := filepath.Join(t.TempDir(), "malformed.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o600))
	require.ErrorContains(t, runCleanupApply(cmd, bad), "parsing cleanup plan")
}

func TestResolveRemovalWorktreeIdentityBranches(t *testing.T) {
	repoDir, workspacePath := initCLITestRepo(t), filepath.Join(t.TempDir(), "workspace")
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repo, err := store.CreateRepository(repoDir)
	require.NoError(t, err)
	workspace, err := store.CreateWorkspace(repo.ID, "repo/resolve", "git", "resolve", workspacePath)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)
	wm := newManager(vcs.KindGit)

	old := removalIdentityStoreFactory
	defer func() { removalIdentityStoreFactory = old }()
	removalIdentityStoreFactory = func() (removalIdentityStore, error) { return store, nil }
	resolved, err := resolveRemovalWorktree(repoDir, workspace.ID, wm)
	require.NoError(t, err)
	require.Equal(t, workspacePath, resolved.Path)
	require.Equal(t, workspace.ID, resolved.WorkspaceID)

	workspace.Backend = "jj"
	// Persisting a mismatched backend is intentionally avoided; use a wrapper
	// that returns the altered identity while retaining the real store behavior.
	removalIdentityStoreFactory = func() (removalIdentityStore, error) {
		return alteredWorkspaceStore{removalIdentityStore: store, workspace: workspace}, nil
	}
	_, err = resolveRemovalWorktree(repoDir, workspace.ID, wm)
	require.ErrorContains(t, err, "belongs to jj")
}

type alteredWorkspaceStore struct {
	removalIdentityStore
	workspace identity.Workspace
}

func (s alteredWorkspaceStore) LookupWorkspace(string) (identity.Workspace, error) {
	return s.workspace, nil
}

func TestFindGlobalStrictAmbiguousAndNotFound(t *testing.T) {
	first := initCLITestRepo(t)
	second := initCLITestRepo(t)
	for _, repo := range []string{first, second} {
		path := filepath.Join(t.TempDir(), "same")
		cmd := execGit(repo, "worktree", "add", "-q", "-b", "same", path)
		require.NoError(t, cmd.Run())
	}
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	matches, err := findGlobalStrict(cmd, []string{first, second}, "same")
	require.NoError(t, err)
	require.Len(t, matches, 2)
	matches, err = findGlobalStrict(cmd, []string{first, second}, "does-not-exist")
	require.NoError(t, err)
	require.Empty(t, matches)
}

func execGit(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func TestRunOnSwitchHooksIsSafeNoOp(t *testing.T) {
	runOnSwitchHooks(&cobra.Command{}, t.TempDir(), "branch")
}
