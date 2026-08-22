package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/stretchr/testify/require"
)

func TestResourcesAndDoctorJSONAreReadOnly(t *testing.T) {
	defer func() { jsonOutput = false; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)
	before, err := os.ReadDir(home)
	require.NoError(t, err)

	for _, name := range []string{"resources", "doctor"} {
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs([]string{name, "--json"})
		require.NoError(t, Execute())
		var report struct {
			Version int `json:"version"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))
		require.Equal(t, ReportVersion, report.Version)
	}

	after, err := os.ReadDir(home)
	require.NoError(t, err)
	require.Equal(t, before, after, "reports must not create identity state")
	_, err = os.Stat(filepath.Join(home, "state.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestResourcesCommandUsesMainManifestAndV11Leases(t *testing.T) {
	home, main, workspace := t.TempDir(), filepath.Join(t.TempDir(), "main"), filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.MkdirAll(main, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(main, ".wtf.toml"), []byte("version = 1\n[[resources.files]]\nname = \"env\"\nsource = \".env\"\ntarget = \".env\"\nmode = \"symlink\"\nsecret = true\n[resources.ports.web]\npreferred = 3000\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(main, ".env"), []byte("not inspected"), 0o600))
	gitRun := func(args ...string) { c := exec.Command("git", args...); c.Dir = main; require.NoError(t, c.Run()) }
	gitRun("init", "-q")
	gitRun("config", "user.email", "test@example.com")
	gitRun("config", "user.name", "test")
	gitRun("add", ".")
	gitRun("commit", "-qm", "initial")
	gitRun("worktree", "add", "-q", "-b", "test", workspace)
	require.NoError(t, os.Remove(filepath.Join(workspace, ".env")))
	require.NoError(t, os.Symlink(filepath.Join(main, ".env"), filepath.Join(workspace, ".env")))
	const repoID = "11111111-1111-4111-8111-111111111111"
	const workspaceID = "22222222-2222-4222-8222-222222222222"
	now := "2024-01-01T00:00:00Z"
	state := identity.State{Version: identity.StateVersion, Repositories: []identity.Repository{{ID: repoID, Locator: main, LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now}}, Workspaces: []identity.Workspace{{ID: workspaceID, RepositoryID: repoID, Name: "test/workspace", Backend: "git", NativeName: "test", Path: workspace, LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now}}}
	data, err := json.MarshalIndent(state, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(home, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "state.json"), data, 0o600))
	legacy := filepath.Join(main, ".git", "wtf", "ports.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o755))
	require.NoError(t, os.WriteFile(legacy, []byte(`{"test":3000}`), 0o600))
	defer func() { jsonOutput = false; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	t.Setenv("WTF_HOME", home)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"resources", workspaceID, "--json"})
	require.NoError(t, Execute(), out.String())
	var report resourcesReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	item := report.Resources[workspaceID]
	require.Equal(t, resource.ObservedPresent, item.Observed[0].State)
	foundUnavailable := false
	for _, f := range report.Findings {
		foundUnavailable = foundUnavailable || f.Code == "port_lease_unavailable"
	}
	require.True(t, foundUnavailable, "legacy ports.json must not satisfy a v0.11 lease")

	out.Reset()
	rootCmd.SetArgs([]string{"doctor", workspaceID, "--json"})
	require.NoError(t, Execute(), out.String())
	var doctor doctorReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &doctor))
	foundUnavailable = false
	for _, f := range doctor.Findings {
		require.NotEqual(t, "managed_file_broken", f.Code)
		foundUnavailable = foundUnavailable || f.Code == "port_lease_unavailable"
	}
	require.True(t, foundUnavailable, "doctor must not treat legacy ports.json as a v0.11 lease")
}
