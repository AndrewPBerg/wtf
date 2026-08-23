package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCompletionInstall_AllSupportedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			out := new(bytes.Buffer)
			rootCmd.SetOut(out)

			err := runCompletionInstall(rootCmd, shell)
			require.NoError(t, err)
			assert.Contains(t, out.String(), "Installed completions")
		})
	}
}

func TestRunCompletionInstall_InvalidShell(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	err := runCompletionInstall(cmd, "powershell")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--install does not support")
}

type portContractManager struct {
	identityTestManager
	ref        string
	currentErr error
	stateErr   error
}

func (m *portContractManager) CurrentRef(string) (string, error) { return m.ref, m.currentErr }
func (m *portContractManager) StateDir(string) (string, error) {
	if m.stateErr != nil {
		return "", m.stateErr
	}
	return m.stateDir, nil
}

func TestRunPort_ErrorsAndJSON(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := runPort(portCmd)
		require.Error(t, err)
	})

	t.Run("allocator error", func(t *testing.T) {
		dir := initCLITestRepo(t)
		t.Chdir(dir)
		// StateDir is a file, so the port file path cannot be opened.
		state := filepath.Join(dir, ".git", "wtf")
		require.NoError(t, os.WriteFile(state, []byte("not a directory"), 0o600))
		err := runPort(portCmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "allocating port")
	})

	t.Run("json output", func(t *testing.T) {
		dir := initCLITestRepo(t)
		t.Chdir(dir)
		old := jsonOutput
		jsonOutput = true
		t.Cleanup(func() { jsonOutput = old })
		out := new(bytes.Buffer)
		cmd := &cobra.Command{}
		cmd.SetOut(out)
		require.NoError(t, runPort(cmd))
		assert.Contains(t, out.String(), `"branch"`)
	})
}

func TestAllocatePortForWorktree_NonFatalBranches(t *testing.T) {
	cmd := &cobra.Command{}
	stderr := new(bytes.Buffer)
	cmd.SetErr(stderr)

	t.Run("current ref error", func(t *testing.T) {
		stderr.Reset()
		mgr := &portContractManager{currentErr: errors.New("ref failed")}
		allocatePortForWorktree(cmd, mgr, t.TempDir(), t.TempDir())
		assert.Empty(t, stderr.String())
	})

	t.Run("allocator error", func(t *testing.T) {
		stderr.Reset()
		repo := t.TempDir()
		state := filepath.Join(repo, "state")
		require.NoError(t, os.WriteFile(state, []byte("file"), 0o600))
		mgr := &portContractManager{identityTestManager: identityTestManager{stateDir: state}, ref: "feature"}
		allocatePortForWorktree(cmd, mgr, repo, t.TempDir())
		assert.Contains(t, stderr.String(), "port allocation failed")
	})

	t.Run("success without serving", func(t *testing.T) {
		stderr.Reset()
		old := newNoServe
		newNoServe = true
		t.Cleanup(func() { newNoServe = old })
		mgr := &portContractManager{identityTestManager: identityTestManager{stateDir: t.TempDir()}, ref: "feature"}
		allocatePortForWorktree(cmd, mgr, t.TempDir(), t.TempDir())
		assert.Contains(t, stderr.String(), "PORT=")
	})
}

func TestIdentityDependencyWrappersDelegate(t *testing.T) {
	store, err := identity.NewStore(t.TempDir())
	require.NoError(t, err)
	resolver := storeRepositoryResolver{store: store}
	repo, err := resolver.ResolveRepository(t.TempDir(), t.TempDir())
	require.NoError(t, err)

	lifecycle := storeIdentityLifecycle{store: store}
	workspace, err := lifecycle.CreateWorkspace(repo.ID, "repo/feature", string(identity.Git), "feature", "/tmp/feature")
	require.NoError(t, err)
	got, err := lifecycle.LookupWorkspace(workspace.ID)
	require.NoError(t, err)
	assert.Equal(t, workspace.ID, got.ID)
	got, err = lifecycle.MoveWorkspace(workspace.ID, "/tmp/moved")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/moved", got.Path)
	got, err = lifecycle.ActivateWorkspace(workspace.ID)
	require.NoError(t, err)
	assert.Equal(t, identity.Active, got.LifecycleState)
	got, err = lifecycle.MarkCleanupFailed(workspace.ID)
	require.NoError(t, err)
	assert.Equal(t, identity.CleanupFailed, got.LifecycleState)
	active, err := lifecycle.CreateWorkspace(repo.ID, "repo/other", string(identity.Git), "other", "/tmp/other")
	require.NoError(t, err)
	got, err = lifecycle.RemoveWorkspace(active.ID)
	require.NoError(t, err)
	assert.Equal(t, identity.Removed, got.LifecycleState)
}

func TestIdentityJSONIncludesOnlyPresentIdentityFields(t *testing.T) {
	assert.Empty(t, identityJSON(vcs.Worktree{}))
	assert.Equal(t, map[string]string{
		"repository_id": "repo-id",
		"workspace_id":  "workspace-id",
		"name":          "repo/feature",
		"native_name":   "feature",
	}, identityJSON(vcs.Worktree{
		RepositoryID: "repo-id",
		WorkspaceID:  "workspace-id",
		Name:         "repo/feature",
		NativeName:   "feature",
	}))
}

func TestSafeHelperBranches(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"0", true}, {" false ", true}, {"NO", true}, {"off", true}, {"yes", false}, {"", false},
	}
	for _, tt := range tests {
		t.Setenv("WTF_SAFE_HELPER_TEST", tt.value)
		assert.Equal(t, tt.want, envDisabled("WTF_SAFE_HELPER_TEST"))
	}

	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	msg, path := newOutputWriters(cmd, true)
	assert.NotNil(t, msg)
	assert.NotNil(t, path)
	msg, path = newOutputWriters(cmd, false)
	assert.NotNil(t, msg)
	assert.Nil(t, path)
}

var _ vcs.Manager = (*portContractManager)(nil)
