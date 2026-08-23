package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDesiredRejectsInvalidEntriesAndScopesDuplicatesByKind(t *testing.T) {
	tests := []struct {
		name    string
		desired Desired
		want    string
	}{
		{"empty file name", Desired{Files: []FileIntent{{Source: "src", Target: "dst", Mode: "symlink"}}}, "resource name must not be empty"},
		{"empty source", Desired{Files: []FileIntent{{Name: "env", Target: "dst", Mode: "symlink"}}}, `file "env" requires source and target`},
		{"empty target", Desired{Files: []FileIntent{{Name: "env", Source: "src", Mode: "symlink"}}}, `file "env" requires source and target`},
		{"invalid mode", Desired{Files: []FileIntent{{Name: "env", Source: "src", Target: "dst", Mode: "move"}}}, `file "env" has invalid mode`},
		{"duplicate file", Desired{Files: []FileIntent{{Name: "env", Source: "a", Target: "a", Mode: "copy"}, {Name: "env", Source: "b", Target: "b", Mode: "symlink"}}}, `duplicate resource "env"`},
		{"empty port name", Desired{Ports: []PortIntent{{Preferred: 1}}}, "resource name must not be empty"},
		{"port below range", Desired{Ports: []PortIntent{{Name: "web", Preferred: 0}}}, `port "web" is out of range`},
		{"port above range", Desired{Ports: []PortIntent{{Name: "web", Preferred: 65536}}}, `port "web" is out of range`},
		{"duplicate port", Desired{Ports: []PortIntent{{Name: "web", Preferred: 1}, {Name: "web", Preferred: 2}}}, `duplicate resource "web"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.EqualError(t, validateDesired(tt.desired), tt.want)
		})
	}

	// A file and port may intentionally share a display name: their kinds are
	// part of the resource identity.
	require.NoError(t, validateDesired(Desired{
		Files: []FileIntent{{Name: "shared", Source: "a", Target: "b", Mode: "symlink"}},
		Ports: []PortIntent{{Name: "shared", Preferred: 1}},
	}))
}

func TestSortObservedAndLeases(t *testing.T) {
	observed := []Observed{{Kind: KindPort, Name: "z"}, {Kind: KindFile, Name: "z"}, {Kind: KindFile, Name: "a"}}
	sortObserved(observed)
	require.Equal(t, []Observed{{Kind: KindFile, Name: "a"}, {Kind: KindFile, Name: "z"}, {Kind: KindPort, Name: "z"}}, observed)

	leases := []Lease{{Kind: KindPort, Name: "z"}, {Kind: KindFile, Name: "z"}, {Kind: KindFile, Name: "a"}}
	sortLeases(leases)
	require.Equal(t, []Lease{{Kind: KindFile, Name: "a"}, {Kind: KindFile, Name: "z"}, {Kind: KindPort, Name: "z"}}, leases)
}

func TestLoadRejectsMalformedWorkspaceState(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	base := Workspace{Version: Version, WorkspaceID: id, Lifecycle: LifecycleActive}
	tests := []struct {
		name string
		edit func(*Workspace)
		want string
	}{
		{"workspace id does not match key", func(w *Workspace) { w.WorkspaceID = "wrong" }, "invalid workspace record"},
		{"workspace version", func(w *Workspace) { w.Version = 2 }, "invalid workspace record"},
		{"workspace id has another canonical value", func(w *Workspace) { w.WorkspaceID = "00000000-0000-0000-0000-000000000002" }, "invalid workspace record"},
		{"lifecycle", func(w *Workspace) { w.Lifecycle = "broken" }, "invalid lifecycle"},
		{"observed kind", func(w *Workspace) { w.Observed = []Observed{{Kind: "bad", Name: "x", State: ObservedPresent}} }, "unsupported resource kind"},
		{"observed name", func(w *Workspace) { w.Observed = []Observed{{Kind: KindFile, State: ObservedPresent}} }, "resource name must not be empty"},
		{"observed state", func(w *Workspace) { w.Observed = []Observed{{Kind: KindFile, Name: "x", State: "bad"}} }, "invalid observed state"},
		{"lease id", func(w *Workspace) { w.Leases = []Lease{{Kind: KindPort, Name: "x", State: LeaseAcquired}} }, "invalid lease"},
		{"lease state", func(w *Workspace) { w.Leases = []Lease{{Kind: KindPort, Name: "x", ID: "lease", State: "bad"}} }, "invalid lease"},
		{"ownership", func(w *Workspace) { w.FileOwnership = []FileOwnership{{Name: "x", Target: "x", Mode: "bad"}} }, "invalid file ownership"},
		{"copy checksum", func(w *Workspace) { w.FileOwnership = []FileOwnership{{Name: "x", Target: "x", Mode: "copy"}} }, "lacks a checksum"},
		{"cleanup kind", func(w *Workspace) { w.CleanupDebt = []CleanupDebt{{Kind: "bad", Name: "x", Reason: "why"}} }, "unsupported resource kind"},
		{"cleanup reason", func(w *Workspace) { w.CleanupDebt = []CleanupDebt{{Kind: KindFile, Name: "x"}} }, "reason must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := testID(t)
			w := base
			w.WorkspaceID = id
			tt.edit(&w)
			path := filepath.Join(t.TempDir(), "resources.json")
			data, err := json.Marshal(registryFile{Version: Version, Workspaces: map[string]Workspace{id: w}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))
			_, err = NewRegistry(path).Load()
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestRegistryLockAndReleaseValidationErrors(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))
	r := NewRegistry(filepath.Join(parent, "resources.json"))
	require.ErrorContains(t, r.SetDesired(testID(t), desired()), "creating resource registry directory")

	r = NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	require.EqualError(t, r.Release("", KindPort, "web", "x"), `workspace ID: invalid canonical UUID ""`)
	require.EqualError(t, r.Release(id, Kind("bad"), "web", "x"), `unsupported resource kind "bad"`)
	require.EqualError(t, r.Release(id, KindPort, "", "x"), "resource name must not be empty")
}
