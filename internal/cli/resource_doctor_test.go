package cli

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/stretchr/testify/require"
)

func coverageGitWorkspace(t *testing.T) (identity.Repository, identity.Workspace) {
	t.Helper()
	repoDir := initCLITestRepo(t)
	home := os.Getenv("WTF_HOME")
	store, err := identity.NewStore(home)
	require.NoError(t, err)
	repo, err := store.CreateRepository(repoDir)
	require.NoError(t, err)
	workspacePath := filepath.Join(t.TempDir(), "checkout")
	cmd := exec.Command("git", "worktree", "add", "-q", "-b", "coverage", workspacePath)
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	workspace, err := store.CreateWorkspace(repo.ID, "coverage/test", "git", "coverage", workspacePath)
	require.NoError(t, err)
	workspace, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)
	return repo, workspace
}

func TestObserveResourcesReportsMissingDriftAndPortDebtDeterministically(t *testing.T) {
	repo, workspace := coverageGitWorkspace(t)
	mgr := git.NewWorktreeManager(&git.RealExecutor{})
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, occupied.Close()) }()
	port := occupied.Addr().(*net.TCPAddr).Port

	desired := resource.Desired{
		Files: []resource.FileIntent{
			{Name: "missing", Source: ".env", Target: ".missing", Mode: "symlink"},
			{Name: "drift", Source: ".env", Target: ".drift", Mode: "symlink"},
		},
		Ports: []resource.PortIntent{{Name: "web", Preferred: port}},
	}
	require.NoError(t, os.Symlink(filepath.Join(repo.Locator, ".wrong"), filepath.Join(workspace.Path, ".drift")))
	observed, findings := observeResources(workspace, repo, mgr, desired, nil)

	require.Equal(t, []resource.Observed{
		{Kind: resource.KindFile, Name: "missing", State: resource.ObservedAbsent},
		{Kind: resource.KindFile, Name: "drift", State: resource.ObservedInvalid},
		{Kind: resource.KindPort, Name: "web", State: resource.ObservedAbsent},
	}, observed)
	codes := findingCodes(findings)
	require.Equal(t, []string{"managed_file_missing", "managed_file_broken", "port_lease_unavailable"}, codes)
}

func TestCollectResourceReportsFindingsAndRegistryErrors(t *testing.T) {
	repo, workspace := coverageGitWorkspace(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	stateDir, err := git.NewWorktreeManager(&git.RealExecutor{}).StateDir(repo.Locator)
	require.NoError(t, err)
	registryPath := filepath.Join(stateDir, "resources.json")
	reg := resource.NewRegistry(registryPath)
	require.NoError(t, reg.SetDesired(workspace.ID, resource.Desired{Files: []resource.FileIntent{{Name: "env", Source: ".env", Target: ".env", Mode: "symlink"}}}))
	// Corrupting the registry exercises the read-only registry error finding.
	require.NoError(t, os.WriteFile(registryPath, []byte("{not json"), 0o600))

	items, findings, err := collectResourceReports(store, []string{workspace.ID})
	require.NoError(t, err)
	require.Contains(t, items, workspace.ID)
	require.Contains(t, findingCodes(findings), "resource_registry_unavailable")
}

func TestDoctorWorkspacesSortsCleanupAndResourceDebtFindings(t *testing.T) {
	repo, workspace := coverageGitWorkspace(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	workspace, err = store.MarkCleanupFailed(workspace.ID)
	require.NoError(t, err)
	stateDir, err := git.NewWorktreeManager(&git.RealExecutor{}).StateDir(repo.Locator)
	require.NoError(t, err)
	registryPath := filepath.Join(stateDir, "resources.json")
	// Keep this fixture in the registry's public JSON contract while recording
	// two debts in reverse order; doctor must return stable sorted findings.
	raw := map[string]any{"version": 1, "workspaces": map[string]any{workspace.ID: map[string]any{
		"version": 1, "workspace_id": workspace.ID, "lifecycle": "cleanup_failed",
		"cleanup_debt": []resource.CleanupDebt{
			{Kind: resource.KindPort, Name: "web", Reason: "busy"},
			{Kind: resource.KindFile, Name: "env", Reason: "changed"},
		},
	}}}
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(registryPath), 0o700))
	require.NoError(t, os.WriteFile(registryPath, data, 0o600))

	findings, err := doctorWorkspaces([]identity.Workspace{workspace})
	require.NoError(t, err)
	codes := findingCodes(findings)
	require.Equal(t, []string{"cleanup_failed", "resource_cleanup_debt", "resource_cleanup_debt"}, codes)
	require.Equal(t, "file env cleanup debt: changed", findings[1].Message)
	require.Equal(t, "port web cleanup debt: busy", findings[2].Message)
}

func TestPathMissingAndReadCleanupPlanErrors(t *testing.T) {
	t.Setenv("WTF_HOME", t.TempDir())
	existing := filepath.Join(t.TempDir(), "exists")
	require.NoError(t, os.WriteFile(existing, nil, 0o600))
	require.False(t, pathMissing(existing))
	require.True(t, pathMissing(filepath.Join(t.TempDir(), "missing")))

	_, err := readCleanupPlan(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{}"), 0o600))
	_, err = readCleanupPlan(bad)
	require.Error(t, err)
	altered := filepath.Join(t.TempDir(), "altered.json")
	plan := CleanupPlan{Version: 1, PlanID: "not-the-digest"}
	data, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(altered, data, 0o600))
	_, err = readCleanupPlan(altered)
	require.Error(t, err)
}

func findingCodes(findings []Finding) []string {
	codes := make([]string, len(findings))
	for i, f := range findings {
		codes[i] = f.Code
	}
	return codes
}
