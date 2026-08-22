package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/stretchr/testify/require"
)

func commitManifest(t *testing.T, dir, manifest string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wtf.toml"), []byte(manifest), 0o644))
	x := &git.RealExecutor{}
	_, err := x.Run(dir, "add", ".")
	require.NoError(t, err)
	_, err = x.Run(dir, "commit", "-m", "manifest")
	require.NoError(t, err)
}

func createLifecycleWorkspace(t *testing.T, dir string) string {
	t.Helper()
	t.Chdir(dir)
	cmd := newCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	require.NoError(t, runNew(cmd, "lifecycle", "main", wm, nil, false))
	worktrees, err := wm.List(dir)
	require.NoError(t, err)
	for _, wt := range worktrees {
		if wt.Branch == "lifecycle" {
			return wt.Path
		}
	}
	t.Fatal("created lifecycle worktree not found")
	return ""
}

func TestCLILifecycleManifestCreateOwnsFileAndPortLease(t *testing.T) {
	dir := initCLITestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.main"), []byte("SECRET=present\n"), 0o600))
	commitManifest(t, dir, "version = 1\n[[resources.files]]\nname = \"env\"\nsource = \".env.main\"\ntarget = \".env\"\nmode = \"symlink\"\nsecret = false\n[resources.ports.web]\npreferred = 3000\n")
	workspace := createLifecycleWorkspace(t, dir)
	link, err := os.Readlink(filepath.Join(workspace, ".env"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, ".env.main"), link)
	mgr := git.NewWorktreeManager(&git.RealExecutor{})
	stateDir, err := mgr.StateDir(dir)
	require.NoError(t, err)
	store := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
	ws, err := store.Load()
	require.NoError(t, err)
	require.Len(t, ws, 1)
	for _, record := range ws {
		found := false
		for _, lease := range record.Leases {
			if lease.Kind == resource.KindPort && lease.Name == "web" && lease.State == resource.LeaseAcquired && lease.ID != "" {
				found = true
			}
		}
		require.True(t, found)
	}
}

func TestCLILifecycleRefusesUnmanagedTargetUnchanged(t *testing.T) {
	dir := initCLITestRepo(t)
	commitManifest(t, dir, "version = 1\n[[resources.files]]\nname = \"existing\"\nsource = \"init.txt\"\ntarget = \"init.txt\"\nmode = \"symlink\"\nsecret = false\n")
	original := filepath.Join(dir, "init.txt")
	before, err := os.ReadFile(original)
	require.NoError(t, err)
	t.Chdir(dir)
	cmd := newCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.Error(t, runNew(cmd, "unmanaged", "main", git.NewWorktreeManager(&git.RealExecutor{}), nil, false))
	after, err := os.ReadFile(original)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCLILifecycleCleanupErrorRetainsDebtAndIdentityFailed(t *testing.T) {
	dir := initCLITestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.main"), []byte("SECRET=present\n"), 0o600))
	commitManifest(t, dir, "version = 1\n[[resources.files]]\nname = \"env\"\nsource = \".env.main\"\ntarget = \".env\"\nmode = \"symlink\"\nsecret = false\n")
	workspace := createLifecycleWorkspace(t, dir)
	require.NoError(t, os.Remove(filepath.Join(workspace, ".env")))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, ".env"), []byte("unmanaged"), 0o600))
	t.Chdir(dir)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	require.Error(t, runRm(rmCmd, "lifecycle", wm))
	stateDir, err := wm.StateDir(dir)
	require.NoError(t, err)
	records, err := resource.NewRegistry(filepath.Join(stateDir, "resources.json")).Load()
	require.NoError(t, err)
	require.Len(t, records, 1)
	for _, record := range records {
		require.Equal(t, resource.LifecycleCleanupFailed, record.Lifecycle)
		require.NotEmpty(t, record.CleanupDebt)
	}
	is, err := identity.DefaultStore()
	require.NoError(t, err)
	state, err := is.Load()
	require.NoError(t, err)
	found := false
	for _, ws := range state.Workspaces {
		if ws.Path == workspace {
			require.Equal(t, identity.CleanupFailed, ws.LifecycleState)
			found = true
		}
	}
	require.True(t, found)
}

func TestCLILifecycleGlobFailsBeforeResourceWrites(t *testing.T) {
	dir := initCLITestRepo(t)
	commitManifest(t, dir, "version = 1\n[[resources.files]]\nname = \"glob\"\nsource = \"*.env\"\ntarget = \".env\"\nmode = \"symlink\"\nsecret = false\n")
	t.Chdir(dir)
	cmd := newCmd
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.Error(t, runNew(cmd, "glob", "main", git.NewWorktreeManager(&git.RealExecutor{}), nil, false))
	mgr := git.NewWorktreeManager(&git.RealExecutor{})
	stateDir, err := mgr.StateDir(dir)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(stateDir, "resources.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
