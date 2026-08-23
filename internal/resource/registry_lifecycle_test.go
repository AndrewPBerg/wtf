package resource

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanupDebtDuplicateAndOrdering(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))

	require.EqualError(t, r.MarkCleanupDebt(id, KindFile, "secret", ""), "cleanup reason must not be empty")
	require.NoError(t, r.MarkCleanupDebt(id, KindPort, "web", "port busy"))
	require.NoError(t, r.MarkCleanupDebt(id, KindFile, "secret", "permission denied"))
	require.NoError(t, r.MarkCleanupDebt(id, KindFile, "secret", "different reason"))
	w := mustWorkspace(t, r, id)
	require.Equal(t, []CleanupDebt{
		{Kind: KindFile, Name: "secret", Reason: "permission denied"},
		{Kind: KindPort, Name: "web", Reason: "port busy"},
	}, w.CleanupDebt)
	require.Equal(t, LifecycleCleanupFailed, w.Lifecycle)

	require.NoError(t, r.FinalizeCleanup(id, KindFile, "secret"))
	require.Equal(t, LifecycleCleanupFailed, mustWorkspace(t, r, id).Lifecycle)
	require.EqualError(t, r.FinalizeCleanup(id, KindFile, "secret"), "cleanup debt for file/secret not found")
	require.NoError(t, r.FinalizeCleanup(id, KindPort, "web"))
	require.Equal(t, LifecycleActive, mustWorkspace(t, r, id).Lifecycle)
	require.EqualError(t, r.FinalizeCleanup(id, KindPort, "web"), "cleanup debt for port/web not found")
}

func TestCleanupDebtValidationAndMissingWorkspace(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	missing := testID(t)
	require.EqualError(t, r.MarkCleanupDebt("", KindFile, "x", "reason"), `workspace ID: invalid canonical UUID ""`)
	require.EqualError(t, r.MarkCleanupDebt(missing, Kind("bad"), "x", "reason"), `unsupported resource kind "bad"`)
	require.EqualError(t, r.MarkCleanupDebt(missing, KindFile, "", "reason"), "resource name must not be empty")
	require.EqualError(t, r.MarkCleanupDebt(missing, KindFile, "x", "reason"), `workspace "`+missing+`" not found`)
	require.EqualError(t, r.FinalizeCleanup("", KindFile, "x"), `workspace ID: invalid canonical UUID ""`)
	require.EqualError(t, r.FinalizeCleanup(missing, KindFile, "x"), `workspace "`+missing+`" not found`)
}

func TestSortDebtOrdersByKindAndName(t *testing.T) {
	debt := []CleanupDebt{
		{Kind: KindPort, Name: "z"},
		{Kind: KindFile, Name: "z"},
		{Kind: KindFile, Name: "a"},
	}
	sortDebt(debt)
	require.Equal(t, []CleanupDebt{
		{Kind: KindFile, Name: "a"},
		{Kind: KindFile, Name: "z"},
		{Kind: KindPort, Name: "z"},
	}, debt)
}

func TestRegistryLoadRejectsCorruptionVersionsAndUnknownFields(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"malformed JSON", "{", "decoding resource registry"},
		{"wrong registry version", `{"version":2,"workspaces":{}}`, "resource registry version must be 1"},
		{"missing workspaces", `{"version":1}`, "resource registry workspaces is required"},
		{"unknown top-level field", `{"version":1,"workspaces":{},"extra":true}`, "decoding resource registry"},
		{"unknown workspace field", `{"version":1,"workspaces":{"00000000-0000-0000-0000-000000000001":{"version":1,"workspace_id":"00000000-0000-0000-0000-000000000001","lifecycle":"active","desired":{},"extra":true}}}`, "decoding resource registry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resources.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.data), 0o600))
			_, err := NewRegistry(path).Load()
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestGetAndSetDesiredErrorBranches(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	missing := testID(t)
	require.EqualError(t, r.SetDesired("", desired()), `workspace ID: invalid canonical UUID ""`)
	require.EqualError(t, r.SetDesired(testID(t), Desired{Files: []FileIntent{{Name: "x", Source: "", Target: "x", Mode: "symlink"}}}), `file "x" requires source and target`)
	_, err := r.Get("")
	require.EqualError(t, err, `workspace ID: invalid canonical UUID ""`)
	_, err = r.Get(missing)
	require.EqualError(t, err, `workspace "`+missing+`" not found`)

	directory := filepath.Join(t.TempDir(), "registry-dir")
	require.NoError(t, os.Mkdir(directory, 0o700))
	dirRegistry := NewRegistry(directory)
	require.ErrorContains(t, func() error { _, err := dirRegistry.Get(missing); return err }(), "reading resource registry")
	require.Error(t, dirRegistry.SetDesired(testID(t), desired()))
}

func TestLeaseDeclarationAndReleaseEdges(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	id := testID(t)
	require.NoError(t, r.SetDesired(id, desired()))

	fileLease, err := r.Acquire(id, KindFile, "secret")
	require.NoError(t, err)
	require.Equal(t, KindFile, fileLease.Kind)
	_, err = r.Acquire(id, KindFile, "secret")
	require.EqualError(t, err, "resource file/secret is already acquired")
	require.NoError(t, r.Release(id, KindFile, "secret", fileLease.ID))
	second, err := r.Acquire(id, KindFile, "secret")
	require.NoError(t, err)
	require.NotEqual(t, fileLease.ID, second.ID)

	_, err = r.Acquire(id, KindFile, "")
	require.EqualError(t, err, "resource name must not be empty")
	require.EqualError(t, r.Release(id, KindFile, "secret", "unknown"), `lease "unknown" not found`)
	missing := testID(t)
	require.EqualError(t, r.Release(missing, KindPort, "web", "unknown"), `workspace "`+missing+`" not found`)
}
