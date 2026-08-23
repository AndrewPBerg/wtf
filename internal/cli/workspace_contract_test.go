package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentWorkspaceSelectsDeepestEligibleWorkspace(t *testing.T) {
	repo := initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repository, err := store.CreateRepository(repo)
	require.NoError(t, err)
	outer := filepath.Join(repo, "outer")
	inner := filepath.Join(outer, "inner")
	for _, path := range []string{outer, inner} {
		require.NoError(t, os.MkdirAll(path, 0o755))
	}
	first, err := store.CreateWorkspace(repository.ID, "repo/outer", "git", "outer", outer)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(first.ID)
	require.NoError(t, err)
	second, err := store.CreateWorkspace(repository.ID, "repo/inner", "git", "inner", inner)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(second.ID)
	require.NoError(t, err)
	removed, err := store.CreateWorkspace(repository.ID, "repo/removed", "git", "removed", filepath.Join(repo, "removed"))
	require.NoError(t, err)
	_, err = store.RemoveWorkspace(removed.ID)
	require.NoError(t, err)

	old, err := os.Getwd()
	require.NoError(t, err)
	nested := filepath.Join(inner, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.Chdir(nested))
	defer func() { _ = os.Chdir(old) }()

	got, err := currentWorkspace(store)
	require.NoError(t, err)
	assert.Equal(t, second.ID, got.ID)
}

func TestCurrentWorkspaceRejectsUnmanagedDirectory(t *testing.T) {
	initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	defer func() { _ = os.Chdir(old) }()
	_, err = currentWorkspace(store)
	assert.EqualError(t, err, "current directory is not a managed workspace")
}

func TestSelectedWorkspacesSelectorsAndSorting(t *testing.T) {
	initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repository, err := store.CreateRepository(t.TempDir())
	require.NoError(t, err)
	one, err := store.CreateWorkspace(repository.ID, "repo/z", "git", "z", t.TempDir())
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(one.ID)
	require.NoError(t, err)
	two, err := store.CreateWorkspace(repository.ID, "repo/a", "git", "a", t.TempDir())
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(two.ID)
	require.NoError(t, err)
	state, err := store.Load()
	require.NoError(t, err)

	got, err := selectedWorkspaces(state, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Less(t, got[0].ID, got[1].ID)
	assert.ElementsMatch(t, []string{one.ID, two.ID}, []string{got[0].ID, got[1].ID})
	for _, selector := range []string{one.ID, one.Name, one.Path} {
		got, err = selectedWorkspaces(state, []string{selector})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, one.ID, got[0].ID)
	}
	_, err = selectedWorkspaces(state, []string{"does-not-exist"})
	assert.Error(t, err)
	got, err = selectedWorkspaces(state, []string{"one", "two"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestInspectWorkspaceReportsPhysicalGitState(t *testing.T) {
	repo := initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repository, err := store.CreateRepository(repo)
	require.NoError(t, err)
	workspace, err := store.CreateWorkspace(repository.ID, "repo/main", "git", "main", repo)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)

	report, err := inspectWorkspace(workspace)
	require.NoError(t, err)
	assert.Equal(t, ReportVersion, report.Version)
	assert.True(t, report.Physical.Present)
	assert.True(t, report.Physical.PathMatches)
	assert.True(t, report.Physical.IsMain)
	assert.Equal(t, "not_supported", report.GitDiffShadow.Status)
}

func TestInspectWorkspaceErrorsAndUnavailableBackend(t *testing.T) {
	initCLITestRepo(t)
	missingRepo := identity.Workspace{ID: "workspace", RepositoryID: "missing", Backend: "git", Path: t.TempDir()}
	_, err := inspectWorkspace(missingRepo)
	assert.Contains(t, err.Error(), `repository "missing" for workspace "workspace" not found`)

	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repo, err := store.CreateRepository(t.TempDir())
	require.NoError(t, err)
	invalid := identity.Workspace{ID: "workspace", RepositoryID: repo.ID, Backend: "invalid", Path: t.TempDir()}
	_, err = inspectWorkspace(invalid)
	assert.Contains(t, err.Error(), "unknown version control system")
}

func TestInspectGitDiffShadowAdditionalStatuses(t *testing.T) {
	assert.Equal(t, "absent", inspectGitDiffShadow("", "").Status)
	fileGit := filepath.Join(t.TempDir(), ".git")
	require.NoError(t, os.WriteFile(fileGit, []byte("gitdir: nowhere\n"), 0o644))
	assert.Equal(t, "absent", inspectGitDiffShadow(filepath.Dir(fileGit), "").Status)

	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", vcs.JJGitDiffMarker), []byte("shadow\n"), 0o644))
	assert.Equal(t, "stale", inspectGitDiffShadow(repo, "anything").Status)
}

func TestPrintWorkspaceReportIncludesJJAndPhysicalError(t *testing.T) {
	cmd := workspaceInspectCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	printWorkspaceReport(cmd, workspaceReport{
		Identity:      identity.Workspace{ID: "id", Name: "repo/main", Path: "/tmp/repo", Backend: "jj", NativeName: "main", LifecycleState: identity.Active},
		Physical:      physicalReport{Present: true, Error: "  registration mismatch  "},
		JJ:            &jjReport{Workspace: "main", Change: "kxyz", Commit: "abc", Operation: "op1"},
		GitDiffShadow: shadowReport{Supported: true, Status: "present"},
	})
	output := buf.String()
	assert.Contains(t, output, "active (present)")
	assert.Contains(t, output, "jj: workspace=main change=kxyz commit=abc operation=op1")
	assert.Contains(t, output, "physical-error: registration mismatch")
}

func TestWorkspaceCommandsProduceStructuredReports(t *testing.T) {
	repo := initCLITestRepo(t)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	repository, err := store.CreateRepository(repo)
	require.NoError(t, err)
	workspace, err := store.CreateWorkspace(repository.ID, "repo/main", "git", "main", repo)
	require.NoError(t, err)
	_, err = store.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)

	oldJSON := jsonOutput
	jsonOutput = true
	defer func() { jsonOutput = oldJSON }()
	for _, tc := range []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		current bool
	}{
		{"inspect", workspaceInspectCmd, []string{workspace.ID}, false},
		{"list", workspaceListCmd, nil, false},
		{"current", workspaceCurrentCmd, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.current {
				old, chdirErr := os.Getwd()
				require.NoError(t, chdirErr)
				require.NoError(t, os.Chdir(repo))
				defer func() { _ = os.Chdir(old) }()
			}
			buf := new(bytes.Buffer)
			tc.cmd.SetOut(buf)
			tc.cmd.SetArgs(tc.args)
			require.NoError(t, tc.cmd.RunE(tc.cmd, tc.args))
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
			assert.Equal(t, float64(ReportVersion), envelope["version"])
		})
	}
}
