package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func coverageState(t *testing.T) State {
	t.Helper()
	root := t.TempDir()
	repoID, err := NewID()
	require.NoError(t, err)
	workspaceID, err := NewID()
	require.NoError(t, err)
	timestamp := "2024-01-01T00:00:00Z"
	return State{
		Version: StateVersion,
		Repositories: []Repository{{
			ID: repoID, Locator: filepath.Join(root, "repo"), LifecycleState: Active,
			CreatedAt: timestamp, UpdatedAt: timestamp,
		}},
		Workspaces: []Workspace{{
			ID: workspaceID, RepositoryID: repoID, Name: "repo/main", Backend: string(Git),
			NativeName: "main", Path: filepath.Join(root, "workspace"), LifecycleState: Active,
			CreatedAt: timestamp, UpdatedAt: timestamp,
		}},
	}
}

func TestStateValidateInvariantBranches(t *testing.T) {
	base := coverageState(t)
	tests := []struct {
		name string
		edit func(*State)
	}{
		{"version", func(s *State) { s.Version = 2 }},
		{"nil repositories", func(s *State) { s.Repositories = nil }},
		{"nil workspaces", func(s *State) { s.Workspaces = nil }},
		{"repository id", func(s *State) { s.Repositories[0].ID = "bad" }},
		{"duplicate repository id", func(s *State) { s.Repositories = append(s.Repositories, s.Repositories[0]) }},
		{"repository locator", func(s *State) { s.Repositories[0].Locator = "https://example.invalid/repo" }},
		{"repository state", func(s *State) { s.Repositories[0].LifecycleState = "unknown" }},
		{"repository created timestamp", func(s *State) { s.Repositories[0].CreatedAt = "bad" }},
		{"repository updated timestamp", func(s *State) { s.Repositories[0].UpdatedAt = "bad" }},
		{"duplicate active locator", func(s *State) {
			other := s.Repositories[0]
			other.ID, _ = NewID()
			s.Repositories = append(s.Repositories, other)
		}},
		{"workspace id", func(s *State) { s.Workspaces[0].ID = "bad" }},
		{"workspace duplicate identity", func(s *State) { s.Workspaces[0].ID = s.Repositories[0].ID }},
		{"workspace repository", func(s *State) { s.Workspaces[0].RepositoryID = "bad" }},
		{"workspace backend", func(s *State) { s.Workspaces[0].Backend = "svn" }},
		{"workspace native name", func(s *State) { s.Workspaces[0].NativeName = "" }},
		{"workspace created timestamp", func(s *State) { s.Workspaces[0].CreatedAt = "bad" }},
		{"workspace updated timestamp", func(s *State) { s.Workspaces[0].UpdatedAt = "bad" }},
		{"workspace state", func(s *State) { s.Workspaces[0].LifecycleState = "unknown" }},
		{"workspace path", func(s *State) { s.Workspaces[0].Path = "https://example.invalid/workspace" }},
		{"workspace name", func(s *State) { s.Workspaces[0].Name = "main" }},
		{"removed history", func(s *State) { s.Workspaces[0].LifecycleState, s.Workspaces[0].RemovedAt = Removed, "bad" }},
		{"removed cleanup history", func(s *State) { s.Workspaces[0].LifecycleState, s.Workspaces[0].CleanupFailedAt = Removed, "bad" }},
		{"cleanup failed history", func(s *State) { s.Workspaces[0].LifecycleState, s.Workspaces[0].CleanupFailedAt = CleanupFailed, "" }},
		{"cleanup failed removed at", func(s *State) {
			s.Workspaces[0].LifecycleState, s.Workspaces[0].CleanupFailedAt, s.Workspaces[0].RemovedAt = CleanupFailed, "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"
		}},
		{"live removal history", func(s *State) { s.Workspaces[0].RemovedAt = "2024-01-01T00:00:00Z" }},
		{"duplicate claimed name", func(s *State) {
			other := s.Workspaces[0]
			other.ID, _ = NewID()
			other.Path = filepath.Join(t.TempDir(), "other")
			s.Workspaces = append(s.Workspaces, other)
		}},
		{"duplicate claimed path", func(s *State) {
			other := s.Workspaces[0]
			other.ID, _ = NewID()
			other.Name = "repo/other"
			s.Workspaces = append(s.Workspaces, other)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := base
			state.Repositories = append([]Repository(nil), base.Repositories...)
			state.Workspaces = append([]Workspace(nil), base.Workspaces...)
			tc.edit(&state)
			require.Error(t, state.Validate())
		})
	}
}

func TestAdoptExistingWorkspaceAliasAndSafeErrors(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "existing")
	result, err := s.AdoptExistingWorkspace(repo.ID, "repo/existing", string(JJ), "repo/existing", path)
	require.NoError(t, err)
	require.Equal(t, Adopted, result.Status)
	for _, input := range [][5]string{
		{repo.ID, "bad", "jj", "bad", filepath.Join(t.TempDir(), "bad")},
		{repo.ID, "repo/bad", "svn", "bad", filepath.Join(t.TempDir(), "bad2")},
		{repo.ID, "repo/bad", "jj", "other", filepath.Join(t.TempDir(), "bad3")},
	} {
		got, adoptErr := s.AdoptExistingWorkspace(input[0], input[1], input[2], input[3], input[4])
		require.NoError(t, adoptErr)
		require.Equal(t, RenameRequired, got.Status)
	}
	_, err = s.AdoptExistingWorkspace("not-an-id", "repo/missing", string(Git), "main", filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestDefaultStoreOpenAndPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)
	defaultStore, err := DefaultStore()
	require.NoError(t, err)
	opened, err := Open(home)
	require.NoError(t, err)
	statePath, lockPath := opened.Paths()
	require.Equal(t, filepath.Join(home, "state.json"), statePath)
	require.Equal(t, filepath.Join(home, "state.lock"), lockPath)
	defaultState, err := defaultStore.Load()
	require.NoError(t, err)
	require.Equal(t, emptyState(), defaultState)
}

func TestMarkerCorruptionConflictsAndRepair(t *testing.T) {
	s := mustStore(t)
	locator := filepath.Join(t.TempDir(), "repo")
	dir := filepath.Join(t.TempDir(), "state")
	repo, err := s.EnsureRepository(locator, dir)
	require.NoError(t, err)
	require.NoError(t, os.Remove(RepositoryMarkerPath(dir)))
	_, err = s.EnsureRepository(locator, dir)
	require.NoError(t, err)
	other, err := NewID()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(RepositoryMarkerPath(dir), []byte(other+"\n"), 0o600))
	_, err = s.EnsureRepository(locator, dir)
	require.ErrorContains(t, err, "conflicts")
	require.NoError(t, os.WriteFile(RepositoryMarkerPath(dir), []byte(repo.ID+"\n"), 0o600))
	require.Error(t, WriteRepositoryID(dir, other))
	require.NoError(t, os.WriteFile(RepositoryMarkerPath(dir), []byte("bad\r\n"), 0o600))
	_, err = ReadRepositoryID(dir)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(RepositoryMarkerPath(dir), []byte(repo.ID+"\n"), 0o600))
	_, err = s.EnsureRepository(filepath.Join(t.TempDir(), "different"), dir)
	require.ErrorContains(t, err, "mismatch")
}
