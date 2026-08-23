package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/resource"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/stretchr/testify/require"
)

const lifecycleTestID = "11111111-1111-4111-8111-111111111111"

type lifecycleStateManager struct {
	vcs.Manager
	stateDir string
	stateErr error
}

func (m lifecycleStateManager) StateDir(string) (string, error) { return m.stateDir, m.stateErr }

func lifecycleRegistry(t *testing.T) *resource.Registry {
	t.Helper()
	reg := resource.NewRegistry(filepath.Join(t.TempDir(), "resources.json"))
	desired := resource.Desired{Files: []resource.FileIntent{{Name: "env", Source: ".env", Target: ".env", Mode: "copy"}}}
	require.NoError(t, reg.SetDesired(lifecycleTestID, desired))
	return reg
}

func checksumForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestApplyResourceFileSymlinkAndCopy(t *testing.T) {
	mainPath, workspacePath := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "source"), []byte("secret"), 0o600))
	reg := lifecycleRegistry(t)

	symlink := resource.FileIntent{Name: "link", Source: "source", Target: "nested/link", Mode: "symlink"}
	require.NoError(t, applyResourceFile(lifecycleTestID, symlink, mainPath, workspacePath, reg))
	got, err := os.Readlink(filepath.Join(workspacePath, "nested/link"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(mainPath, "source"), got)

	copy := resource.FileIntent{Name: "copy", Source: "source", Target: "nested/copy", Mode: "copy"}
	require.NoError(t, applyResourceFile(lifecycleTestID, copy, mainPath, workspacePath, reg))
	require.Equal(t, []byte("secret"), mustReadFile(t, filepath.Join(workspacePath, "nested/copy")))
	ws, err := reg.Get(lifecycleTestID)
	require.NoError(t, err)
	require.Equal(t, checksumForTest(t, filepath.Join(workspacePath, "nested/copy")), fileOwnership(ws, "copy").Checksum)
}

func TestApplyResourceFileFailures(t *testing.T) {
	reg := lifecycleRegistry(t)
	mainPath, workspacePath := t.TempDir(), t.TempDir()
	missing := resource.FileIntent{Name: "missing", Source: "missing", Target: "missing", Mode: "copy"}
	require.Error(t, applyResourceFile(lifecycleTestID, missing, mainPath, workspacePath, reg))
	require.NoError(t, applyResourceFile(lifecycleTestID, resource.FileIntent{Name: "bad", Source: "missing", Target: "bad", Mode: "symlink"}, mainPath, workspacePath, reg))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "blocked"), []byte("file"), 0o600))
	require.Error(t, applyResourceFile(lifecycleTestID, resource.FileIntent{Name: "blocked", Source: "missing", Target: "blocked/child", Mode: "symlink"}, mainPath, workspacePath, reg))
}

func TestPreflightResourceFilesSafeAndDrift(t *testing.T) {
	mainPath, workspacePath := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "source"), []byte("content"), 0o600))
	reg := lifecycleRegistry(t)
	copy := resource.FileIntent{Name: "copy", Source: "source", Target: "copy", Mode: "copy"}
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "copy"), []byte("content"), 0o600))
	require.NoError(t, reg.RecordFileOwnership(lifecycleTestID, resource.FileOwnership{Name: "copy", Target: "copy", Mode: "copy", Checksum: checksumForTest(t, filepath.Join(workspacePath, "copy"))}))
	require.NoError(t, preflightResourceFiles([]resource.FileIntent{copy}, mainPath, workspacePath, reg, lifecycleTestID))

	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "copy"), []byte("drift"), 0o600))
	require.Error(t, preflightResourceFiles([]resource.FileIntent{copy}, mainPath, workspacePath, reg, lifecycleTestID))
	require.NoError(t, os.Remove(filepath.Join(workspacePath, "copy")))
	require.NoError(t, os.Symlink(filepath.Join(mainPath, "source"), filepath.Join(workspacePath, "link")))
	symlink := resource.FileIntent{Name: "link", Source: "source", Target: "link", Mode: "symlink"}
	require.NoError(t, preflightResourceFiles([]resource.FileIntent{symlink}, mainPath, workspacePath, reg, lifecycleTestID))
	require.NoError(t, os.Remove(filepath.Join(workspacePath, "link")))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "link"), []byte("unmanaged"), 0o600))
	require.Error(t, preflightResourceFiles([]resource.FileIntent{symlink}, mainPath, workspacePath, reg, lifecycleTestID))
	missing := resource.FileIntent{Name: "new", Source: "source", Target: "new", Mode: "copy"}
	require.NoError(t, preflightResourceFiles([]resource.FileIntent{missing}, mainPath, workspacePath, reg, lifecycleTestID))
}

func TestRemoveOwnedFileSafeAndDrift(t *testing.T) {
	mainPath, workspacePath := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "source"), []byte("content"), 0o600))
	desired := resource.Desired{Files: []resource.FileIntent{{Name: "link", Source: "source", Target: "link", Mode: "symlink"}}}
	require.NoError(t, os.Symlink(filepath.Join(mainPath, "source"), filepath.Join(workspacePath, "link")))
	require.NoError(t, removeOwnedFile(resource.FileOwnership{Name: "link", Target: "link", Mode: "symlink"}, desired, mainPath, workspacePath))
	require.NoError(t, os.Symlink(filepath.Join(mainPath, "source"), filepath.Join(workspacePath, "link")))
	require.NoError(t, os.Remove(filepath.Join(workspacePath, "link")))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "other"), filepath.Join(workspacePath, "link")))
	require.Error(t, removeOwnedFile(resource.FileOwnership{Name: "link", Target: "link", Mode: "symlink"}, desired, mainPath, workspacePath))

	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "copy"), []byte("content"), 0o600))
	require.NoError(t, removeOwnedFile(resource.FileOwnership{Name: "copy", Target: "copy", Mode: "copy", Checksum: checksumForTest(t, filepath.Join(workspacePath, "copy"))}, desired, mainPath, workspacePath))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "copy"), []byte("changed"), 0o600))
	require.Error(t, removeOwnedFile(resource.FileOwnership{Name: "copy", Target: "copy", Mode: "copy", Checksum: "wrong"}, desired, mainPath, workspacePath))
}

func TestCleanupResourcesSuccessAndDrift(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "drift"}[drift], func(t *testing.T) {
			mainPath, workspacePath, stateDir := t.TempDir(), t.TempDir(), t.TempDir()
			reg := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
			desired := resource.Desired{Files: []resource.FileIntent{{Name: "link", Source: "source", Target: "link", Mode: "symlink"}}}
			require.NoError(t, reg.SetDesired(lifecycleTestID, desired))
			require.NoError(t, os.WriteFile(filepath.Join(mainPath, "source"), []byte("content"), 0o600))
			require.NoError(t, os.Symlink(filepath.Join(mainPath, "source"), filepath.Join(workspacePath, "link")))
			require.NoError(t, reg.RecordFileOwnership(lifecycleTestID, resource.FileOwnership{Name: "link", Target: "link", Mode: "symlink"}))
			if drift {
				require.NoError(t, os.Remove(filepath.Join(workspacePath, "link")))
				require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "link"), []byte("drift"), 0o600))
			}
			failed := false
			err := cleanupResources(lifecycleTestID, mainPath, workspacePath, lifecycleStateManager{stateDir: stateDir}, func() error { failed = true; return nil })
			if drift {
				require.Error(t, err)
				require.True(t, failed)
				return
			}
			require.NoError(t, err)
			_, statErr := os.Lstat(filepath.Join(workspacePath, "link"))
			require.ErrorIs(t, statErr, os.ErrNotExist)
			ws, getErr := reg.Get(lifecycleTestID)
			require.NoError(t, getErr)
			require.Equal(t, resource.LifecycleFinalized, ws.Lifecycle)
		})
	}
}

func TestCleanupResourcesHandlesAbsentStatePortsAndWorkspaceDebt(t *testing.T) {
	workspacePath := t.TempDir()
	require.NoError(t, cleanupResources(lifecycleTestID, t.TempDir(), workspacePath, lifecycleStateManager{stateDir: t.TempDir()}, func() error {
		t.Fatal("absent resource state marked identity failed")
		return nil
	}))

	stateErr := errors.New("state unavailable")
	require.ErrorIs(t, cleanupResources(lifecycleTestID, t.TempDir(), workspacePath, lifecycleStateManager{stateErr: stateErr}, func() error { return nil }), stateErr)

	stateDir := t.TempDir()
	reg := resource.NewRegistry(filepath.Join(stateDir, "resources.json"))
	desired := resource.Desired{Ports: []resource.PortIntent{{Name: "web", Preferred: 3000}}}
	require.NoError(t, reg.SetDesired(lifecycleTestID, desired))
	_, err := reg.Acquire(lifecycleTestID, resource.KindPort, "web")
	require.NoError(t, err)
	require.NoError(t, cleanupResources(lifecycleTestID, t.TempDir(), workspacePath, lifecycleStateManager{stateDir: stateDir}, func() error { return nil }))
	got, err := reg.Get(lifecycleTestID)
	require.NoError(t, err)
	require.Equal(t, resource.LifecycleFinalized, got.Lifecycle)
	require.Equal(t, resource.LeaseReleased, got.Leases[0].State)

	debtState := t.TempDir()
	debtRegistry := resource.NewRegistry(filepath.Join(debtState, "resources.json"))
	require.NoError(t, debtRegistry.SetDesired(lifecycleTestID, resource.Desired{}))
	require.NoError(t, debtRegistry.MarkCleanupDebt(lifecycleTestID, resource.KindFile, "orphan", "manual repair required"))
	failed := false
	err = cleanupResources(lifecycleTestID, t.TempDir(), workspacePath, lifecycleStateManager{stateDir: debtState}, func() error {
		failed = true
		return nil
	})
	require.ErrorContains(t, err, "finalizing resource workspace")
	require.True(t, failed)
}

func TestDebtExistsAndFileOwnership(t *testing.T) {
	debt := []resource.CleanupDebt{{Kind: resource.KindFile, Name: "env"}}
	require.True(t, debtExists(debt, resource.KindFile, "env"))
	require.False(t, debtExists(debt, resource.KindPort, "env"))
	ws := resource.Workspace{FileOwnership: []resource.FileOwnership{{Name: "env", Target: ".env"}}}
	require.Equal(t, ".env", fileOwnership(ws, "env").Target)
	require.Nil(t, fileOwnership(ws, "missing"))
}

func TestFileChecksumSuccessAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("abc"), 0o600))
	require.Equal(t, checksumForTest(t, path), mustChecksum(t, path))
	_, err := fileChecksum(filepath.Join(filepath.Dir(path), "missing"))
	require.Error(t, err)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
func mustChecksum(t *testing.T, path string) string {
	t.Helper()
	sum, err := fileChecksum(path)
	require.NoError(t, err)
	return sum
}
