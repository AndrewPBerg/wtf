package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

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
