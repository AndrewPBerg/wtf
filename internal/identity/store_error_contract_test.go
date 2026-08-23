package identity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStoreUsesHomeFallback(t *testing.T) {
	t.Setenv("WTF_HOME", "")
	s, err := NewStore("")
	require.NoError(t, err)
	statePath, lockPath := s.Paths()
	require.Equal(t, filepath.Join(s.home, "state.json"), statePath)
	require.Equal(t, filepath.Join(s.home, "state.lock"), lockPath)
	require.NotEmpty(t, s.home)
}

func TestCreateRepositoryRejectsInvalidAndDuplicateLocator(t *testing.T) {
	s := mustStore(t)
	locator := filepath.Join(t.TempDir(), "repo")

	_, err := s.CreateRepository("")
	require.Error(t, err)
	_, err = s.CreateRepository("https://example.invalid/repo")
	require.Error(t, err)
	_, err = s.CreateRepository(locator)
	require.NoError(t, err)
	_, err = s.CreateRepository(locator + string(filepath.Separator) + ".")
	require.ErrorContains(t, err, "already exists")
}

func TestCreateWorkspaceRejectsInvalidInputsAndClaims(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)

	tests := []struct {
		name string
		args func() (string, string, string, string, string)
	}{
		{"repository UUID", func() (string, string, string, string, string) {
			return "bad", "repo/main", "git", "main", filepath.Join(t.TempDir(), "bad-id")
		}},
		{"workspace name", func() (string, string, string, string, string) {
			return repo.ID, "main", "git", "main", filepath.Join(t.TempDir(), "bad-name")
		}},
		{"backend", func() (string, string, string, string, string) {
			return repo.ID, "repo/svn", "svn", "main", filepath.Join(t.TempDir(), "bad-backend")
		}},
		{"native name", func() (string, string, string, string, string) {
			return repo.ID, "repo/empty", "git", "", filepath.Join(t.TempDir(), "bad-native")
		}},
		{"path", func() (string, string, string, string, string) {
			return repo.ID, "repo/remote", "git", "main", "https://example.invalid/workspace"
		}},
		{"missing repository", func() (string, string, string, string, string) {
			return "550e8400-e29b-41d4-a716-446655440000", "repo/missing", "git", "main", filepath.Join(t.TempDir(), "missing-repo")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CreateWorkspace(tc.args())
			require.Error(t, err)
		})
	}

	first, err := s.CreateWorkspace(repo.ID, "repo/main", "git", "main", filepath.Join(t.TempDir(), "first"))
	require.NoError(t, err)
	_, err = s.CreateWorkspace(repo.ID, "repo/other", "git", "other", first.Path)
	require.ErrorContains(t, err, "already claimed")
	_, err = s.CreateWorkspace(repo.ID, first.Name, "git", "other", filepath.Join(t.TempDir(), "second"))
	require.ErrorContains(t, err, "already claimed")
}

func TestCreateWorkspaceRejectsInactiveRepository(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	state, err := s.Load()
	require.NoError(t, err)
	state.Repositories[0].LifecycleState = Removed
	state.Repositories[0].UpdatedAt = state.Repositories[0].CreatedAt
	statePath, _ := s.Paths()
	require.NoError(t, writeState(statePath, state))

	_, err = s.CreateWorkspace(repo.ID, "repo/main", "git", "main", filepath.Join(t.TempDir(), "workspace"))
	require.ErrorContains(t, err, "not active")
}

func TestWorkspaceLifecycleRejectsMissingAndWrongStates(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	pending, err := s.CreateWorkspace(repo.ID, "repo/pending", "git", "pending", filepath.Join(t.TempDir(), "pending"))
	require.NoError(t, err)
	missing, err := NewID()
	require.NoError(t, err)

	_, err = s.ActivateWorkspace(missing)
	require.ErrorContains(t, err, "not found")
	_, err = s.MarkCleanupFailed(missing)
	require.ErrorContains(t, err, "not found")
	_, err = s.FinalizeCleanup(missing)
	require.ErrorContains(t, err, "not found")
	_, err = s.RemoveWorkspace(missing)
	require.ErrorContains(t, err, "not found")

	_, err = s.FinalizeCleanup(pending.ID)
	require.ErrorContains(t, err, "not cleanup_failed")
	active, err := s.ActivateWorkspace(pending.ID)
	require.NoError(t, err)
	_, err = s.ActivateWorkspace(active.ID)
	require.ErrorContains(t, err, "not pending")
	_, err = s.MarkCleanupFailed(active.ID)
	require.NoError(t, err)
	_, err = s.MarkCleanupFailed(active.ID)
	require.ErrorContains(t, err, "already removed")
	_, err = s.RemoveWorkspace(active.ID)
	require.ErrorContains(t, err, "immutable")
	removed, err := s.FinalizeCleanup(active.ID)
	require.NoError(t, err)
	_, err = s.FinalizeCleanup(removed.ID)
	require.ErrorContains(t, err, "not cleanup_failed")
	_, err = s.RemoveWorkspace(removed.ID)
	require.ErrorContains(t, err, "immutable")
}

func TestLookupWorkspaceMissingAndTombstoneSelectors(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "workspace")
	w, err := s.CreateWorkspace(repo.ID, "repo/main", "git", "main", path)
	require.NoError(t, err)
	_, err = s.RemoveWorkspace(w.ID)
	require.NoError(t, err)

	byID, err := s.LookupWorkspace(w.ID)
	require.NoError(t, err)
	require.Equal(t, Removed, byID.LifecycleState)
	_, err = s.LookupWorkspace(w.Name)
	require.ErrorContains(t, err, "not found")
	_, err = s.LookupWorkspace(path)
	require.ErrorContains(t, err, "not found")
	_, err = s.LookupWorkspace("repo/missing")
	require.ErrorContains(t, err, "not found")
	_, err = s.LookupWorkspace("https://example.invalid/workspace")
	require.ErrorContains(t, err, "not found")
	_, err = s.LookupRepository("https://example.invalid/repository")
	require.ErrorContains(t, err, "not found")
}

func TestLoadRejectsReadAndTrailingDataErrors(t *testing.T) {
	s := mustStore(t)
	statePath, _ := s.Paths()
	require.NoError(t, os.Mkdir(statePath, 0o700))
	_, err := s.Load()
	require.Error(t, err)
	require.NoError(t, os.Remove(statePath))
	require.NoError(t, os.WriteFile(statePath, []byte(`{"version":1,"repositories":[],"workspaces":[]} {}`), 0o600))
	_, err = s.Load()
	require.ErrorContains(t, err, "trailing data")
}

func TestStorePersistenceErrorBranches(t *testing.T) {
	_, err := ReadRepositoryID(filepath.Join(t.TempDir(), "missing-marker"))
	require.Error(t, err)
	markerDir := filepath.Join(t.TempDir(), "marker")
	require.NoError(t, os.Mkdir(markerDir, 0o700))
	markerLock := RepositoryMarkerPath(markerDir) + ".lock"
	require.NoError(t, os.Mkdir(markerLock, 0o700))
	markerID, err := NewID()
	require.NoError(t, err)
	require.Error(t, WriteRepositoryID(markerDir, markerID))
	markerWithDirectory := filepath.Join(t.TempDir(), "marker-directory")
	require.NoError(t, os.Mkdir(markerWithDirectory, 0o700))
	require.NoError(t, os.Mkdir(RepositoryMarkerPath(markerWithDirectory), 0o700))
	require.Error(t, WriteRepositoryID(markerWithDirectory, markerID))
	// MkdirAll and lock acquisition failures are deterministic without changing
	// production code: these paths are occupied by files/directories.
	homeFile := filepath.Join(t.TempDir(), "home-file")
	require.NoError(t, os.WriteFile(homeFile, nil, 0o600))
	s, err := NewStore(homeFile)
	require.NoError(t, err)
	_, err = s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.Error(t, err)

	s = mustStore(t)
	require.NoError(t, os.Mkdir(s.lockPath, 0o700))
	_, err = s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.Error(t, err)

	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.Mkdir(target, 0o700))
	err = writeState(target, emptyState())
	require.Error(t, err)
	require.False(t, errors.Is(err, os.ErrNotExist))
	err = writeState(filepath.Join(t.TempDir(), "missing", "state.json"), emptyState())
	require.Error(t, err)
}
