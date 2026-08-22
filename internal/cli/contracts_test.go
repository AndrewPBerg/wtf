package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilitiesJSONCommand(t *testing.T) {
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })
	var out bytes.Buffer
	capabilitiesCmd.SetOut(&out)
	require.NoError(t, capabilitiesCmd.RunE(capabilitiesCmd, nil))

	var report capabilitiesReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, ReportVersion, report.Version)
	assert.Equal(t, []string{"git", "jj"}, report.SupportedVCSBackends)
	assert.Equal(t, []string{"ports", "files"}, report.SupportedResourceKinds)
	assert.Contains(t, report.SupportedDoctorChecks, "git_shadow")
	assert.Equal(t, ReportVersion, report.ResultSchemas["workspace_current"])
}

func TestWorkspaceCurrentJSONCommandUsesContainingWorkspace(t *testing.T) {
	repo := initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repository, err := store.CreateRepository(repo)
	require.NoError(t, err)
	workspace, err := store.CreateWorkspace(repository.ID, "repo/main", "git", "main", repo)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)

	nested := filepath.Join(repo, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(nested))
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })
	var out bytes.Buffer
	workspaceCurrentCmd.SetOut(&out)
	require.NoError(t, workspaceCurrentCmd.RunE(workspaceCurrentCmd, nil))

	var report workspaceReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, ReportVersion, report.Version)
	assert.Equal(t, workspace.ID, report.Identity.ID)
	assert.Equal(t, repository.ID, report.Identity.RepositoryID)
	assert.True(t, report.Physical.Present)
}

func TestFindingUsesSeverityAndOptionalRepairCommand(t *testing.T) {
	data, err := json.Marshal(Finding{Code: "cleanup_failed", Severity: SeverityWarning, RepositoryID: "repo-id", WorkspaceID: "workspace-id", Message: "repair needed"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"code":"cleanup_failed","severity":"warning","repository_id":"repo-id","workspace_id":"workspace-id","message":"repair needed"}`, string(data))
}
