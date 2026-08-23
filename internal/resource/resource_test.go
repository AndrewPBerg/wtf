package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/stretchr/testify/require"
)

func testID(t *testing.T) string {
	t.Helper()
	id, err := identity.NewID()
	require.NoError(t, err)
	return id
}
func desired() Desired {
	return Desired{Files: []FileIntent{{Name: "secret", Source: ".env", Target: ".env", Mode: "symlink", Secret: true}}, Ports: []PortIntent{{Name: "web", Preferred: 3000}}}
}

func TestRegistryStrictDecodeAndNoSecretContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.json")
	id := testID(t)
	r := NewRegistry(path)
	require.NoError(t, r.SetDesired(id, desired()))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotContains(t, string(data), "contents")
	_, err = r.Load()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte("{}")...), 0o600))
	_, err = r.Load()
	require.Error(t, err)
}

func TestRegistryAtomicAcquireRelease(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	var wg sync.WaitGroup
	successes := make(chan Lease, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := r.Acquire(id, KindPort, "web")
			if err == nil {
				successes <- lease
			}
		}()
	}
	wg.Wait()
	close(successes)
	require.Len(t, successes, 1)
	lease := <-successes
	require.NoError(t, r.Release(id, KindPort, "web", lease.ID))
	_, err := r.Acquire(id, KindPort, "web")
	require.NoError(t, err)
}

func TestReconcileIsDeterministic(t *testing.T) {
	plan, err := Reconcile(desired(), []Observed{{Kind: KindFile, Name: "old", State: ObservedPresent}, {Kind: KindPort, Name: "web", State: ObservedPresent}})
	require.NoError(t, err)
	require.Equal(t, Plan{Items: []PlanItem{{Kind: KindFile, Name: "old", Action: ActionRemove}, {Kind: KindFile, Name: "secret", Action: ActionCreate}}}, plan)
}

func TestReconcileRejectsGlobsBeforePlanning(t *testing.T) {
	_, err := Reconcile(Desired{Files: []FileIntent{{Name: "env", Source: "config/*.env", Target: "env", Mode: "symlink"}}}, nil)
	require.EqualError(t, err, `resource glob patterns are not supported during reconciliation: file "env"`)
}

func TestCleanupDebtCanBeFinalized(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	require.NoError(t, r.MarkCleanupDebt(id, KindFile, "secret", "permission denied"))
	w, err := r.Get(id)
	require.NoError(t, err)
	require.Equal(t, LifecycleCleanupFailed, w.Lifecycle)
	require.NoError(t, r.FinalizeCleanup(id, KindFile, "secret"))
	w, err = r.Get(id)
	require.NoError(t, err)
	require.Empty(t, w.CleanupDebt)
	require.Equal(t, LifecycleActive, w.Lifecycle)
}

func TestFromManifestConvertsAndValidates(t *testing.T) {
	tests := []struct {
		name     string
		manifest *config.Manifest
		want     Desired
		err      string
	}{
		{
			name: "converts metadata in declaration order",
			manifest: &config.Manifest{Version: Version, Resources: config.Resources{
				Files: []config.File{{Name: "env", Source: ".env", Target: ".env", Mode: "symlink", Secret: true}},
				Ports: []config.Port{{Name: "web", Preferred: 3000}},
			}},
			want: Desired{
				Files: []FileIntent{{Name: "env", Source: ".env", Target: ".env", Mode: "symlink", Secret: true}},
				Ports: []PortIntent{{Name: "web", Preferred: 3000}},
			},
		},
		{name: "nil manifest", err: "manifest version must be 1"},
		{name: "wrong version", manifest: &config.Manifest{Version: 2}, err: "manifest version must be 1"},
		{
			name:     "invalid file mode",
			manifest: &config.Manifest{Version: Version, Resources: config.Resources{Files: []config.File{{Name: "env", Source: ".env", Target: ".env", Mode: "move"}}}},
			err:      `file "env" has invalid mode`,
		},
		{
			name:     "invalid port",
			manifest: &config.Manifest{Version: Version, Resources: config.Resources{Ports: []config.Port{{Name: "web", Preferred: 0}}}},
			err:      `port "web" is out of range`,
		},
		{
			name: "duplicate file",
			manifest: &config.Manifest{Version: Version, Resources: config.Resources{Files: []config.File{
				{Name: "env", Source: ".env", Target: ".env", Mode: "symlink"},
				{Name: "env", Source: ".env.local", Target: ".env.local", Mode: "copy"},
			}}},
			err: `duplicate resource "env"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromManifest(tt.manifest)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRegistryFileOwnershipRecordReplaceAndRemove(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))

	require.EqualError(t, r.RecordFileOwnership(id, FileOwnership{Name: "env", Target: ".env", Mode: "copy"}), "copy ownership requires a checksum")
	require.EqualError(t, r.RecordFileOwnership(id, FileOwnership{Name: "env", Target: ".env", Mode: "bad"}), "invalid file ownership")
	missingID := testID(t)
	require.EqualError(t, r.RecordFileOwnership(missingID, FileOwnership{Name: "env", Target: ".env", Mode: "symlink"}), `workspace "`+missingID+`" not found`)

	first := FileOwnership{Name: "env", Target: ".env", Mode: "symlink"}
	require.NoError(t, r.RecordFileOwnership(id, first))
	second := FileOwnership{Name: "env", Target: ".env", Mode: "copy", Checksum: "abc"}
	require.NoError(t, r.RecordFileOwnership(id, second))
	w, err := r.Get(id)
	require.NoError(t, err)
	require.Equal(t, []FileOwnership{second}, w.FileOwnership)
	require.EqualError(t, r.RemoveFileOwnership(id, "missing"), `file ownership "missing" not found`)
	require.NoError(t, r.RemoveFileOwnership(id, "env"))
	require.Empty(t, mustWorkspace(t, r, id).FileOwnership)
}

func TestObserveReplacesAndSortsObservations(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	observed := []Observed{{Kind: KindPort, Name: "web", State: ObservedPresent}, {Kind: KindFile, Name: "env", State: ObservedUnknown}}
	require.NoError(t, r.Observe(id, observed))
	w := mustWorkspace(t, r, id)
	require.Equal(t, observed[1], w.Observed[0])
	require.Equal(t, observed[0], w.Observed[1])
	require.NoError(t, r.Observe(id, nil))
	require.Empty(t, mustWorkspace(t, r, id).Observed)
	require.EqualError(t, r.Observe(id, []Observed{{Kind: KindFile, Name: "env", State: "bad"}}), `invalid observed state "bad"`)
	missingID := testID(t)
	require.EqualError(t, r.Observe(missingID, nil), `workspace "`+missingID+`" not found`)
}

func TestFinalizeWorkspaceRejectsCleanupStateAndSucceeds(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	require.NoError(t, r.RecordFileOwnership(id, FileOwnership{Name: "env", Target: ".env", Mode: "symlink"}))
	require.EqualError(t, r.FinalizeWorkspace(id), `workspace "`+id+`" still has resource cleanup state`)
	require.NoError(t, r.RemoveFileOwnership(id, "env"))
	lease, err := r.Acquire(id, KindPort, "web")
	require.NoError(t, err)
	require.EqualError(t, r.FinalizeWorkspace(id), "lease port/web is still acquired")
	require.NoError(t, r.Release(id, KindPort, "web", lease.ID))
	require.NoError(t, r.FinalizeWorkspace(id))
	require.Equal(t, LifecycleFinalized, mustWorkspace(t, r, id).Lifecycle)
	require.EqualError(t, r.SetDesired(id, desired()), `workspace "`+id+`" is finalized`)
}

func TestAcquireAndReleaseRejectInvalidRequests(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))
	_, err := r.Acquire(id, KindPort, "missing")
	require.EqualError(t, err, "resource port/missing is not desired")
	_, err = r.Acquire(id, Kind("bad"), "web")
	require.EqualError(t, err, `unsupported resource kind "bad"`)
	_, err = r.Acquire(testID(t), KindPort, "web")
	require.Contains(t, err.Error(), "workspace ")
	lease, err := r.Acquire(id, KindPort, "web")
	require.NoError(t, err)
	_, err = r.Acquire(id, KindPort, "web")
	require.EqualError(t, err, "resource port/web is already acquired")
	require.EqualError(t, r.Release(id, KindPort, "web", "wrong"), `lease "wrong" not found`)
	require.NoError(t, r.Release(id, KindPort, "web", lease.ID))
	require.NoError(t, r.Release(id, KindPort, "web", lease.ID))
}

func mustWorkspace(t *testing.T, r *Registry, id string) Workspace {
	t.Helper()
	w, err := r.Get(id)
	require.NoError(t, err)
	return w
}
