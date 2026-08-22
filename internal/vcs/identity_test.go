package vcs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/stretchr/testify/require"
)

func TestEnrichWorktreesByCanonicalPathPreservesVCSFields(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "workspace")
	require.NoError(t, os.MkdirAll(realPath, 0o755))
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(realPath, alias))

	now := "2024-01-01T00:00:00Z"
	repositoryID := "550e8400-e29b-41d4-a716-446655440001"
	state := identity.State{Version: identity.StateVersion,
		Repositories: []identity.Repository{{ID: repositoryID, Locator: root, LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now}},
		Workspaces: []identity.Workspace{{
			ID: "550e8400-e29b-41d4-a716-446655440000", RepositoryID: repositoryID,
			Name: "repo/feature", NativeName: "repo/feature", Backend: "jj", Path: realPath,
			LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now,
		}}}
	legacy := Worktree{Path: alias, Branch: "origin/feature", Head: "abc123", VCS: KindJJ, ChangeID: "kxyz", Bookmarks: []string{"feature"}}
	got, err := EnrichWorktrees(state, repositoryID, KindJJ, []Worktree{legacy})
	require.NoError(t, err)
	require.Equal(t, legacy.Branch, got[0].Branch)
	require.Equal(t, legacy.Path, got[0].Path)
	require.Equal(t, legacy.Head, got[0].Head)
	require.Equal(t, legacy.ChangeID, got[0].ChangeID)
	require.Equal(t, legacy.VCS, got[0].VCS)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", got[0].WorkspaceID)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440001", got[0].RepositoryID)
	require.Equal(t, "repo/feature", got[0].Name)
	require.Equal(t, "repo/feature", got[0].NativeName)
}

func TestEnrichLeavesLegacyAndRemovedEntriesUnadopted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy")
	require.NoError(t, os.MkdirAll(path, 0o755))
	state := identity.State{Workspaces: []identity.Workspace{
		{ID: "550e8400-e29b-41d4-a716-446655440000", Path: path, LifecycleState: identity.Removed},
	}}
	got, err := EnrichWorktrees(state, "", KindGit, []Worktree{{Path: path, Branch: "legacy"}})
	require.NoError(t, err)
	require.Empty(t, got[0].WorkspaceID)
	require.Empty(t, got[0].RepositoryID)
	require.Equal(t, "legacy", got[0].Branch)
}

func TestEnrichRejectsInvalidAndConflictingIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "workspace")
	require.NoError(t, os.Mkdir(path, 0o755))
	now := "2024-01-01T00:00:00Z"
	repoID := "550e8400-e29b-41d4-a716-446655440001"
	workspace := identity.Workspace{ID: "550e8400-e29b-41d4-a716-446655440000", RepositoryID: repoID, Name: "repo/feature", Backend: "git", NativeName: "feature", Path: path, LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now}
	state := identity.State{Version: identity.StateVersion, Repositories: []identity.Repository{{ID: repoID, Locator: root, LifecycleState: identity.Active, CreatedAt: now, UpdatedAt: now}}, Workspaces: []identity.Workspace{workspace}}
	_, err := EnrichWorktrees(state, "550e8400-e29b-41d4-a716-446655440002", KindGit, []Worktree{{Path: path, VCS: KindGit}})
	require.Error(t, err)
	got, err := EnrichWorktrees(state, repoID, KindJJ, []Worktree{{Path: path, VCS: KindJJ}})
	require.NoError(t, err)
	require.Empty(t, got[0].WorkspaceID)
	require.Empty(t, got[0].RepositoryID)
	state.Version++
	_, err = EnrichWorktrees(state, repoID, KindGit, []Worktree{{Path: path, VCS: KindGit}})
	require.Error(t, err)

	state.Version = identity.StateVersion
	workspace.Path = filepath.Join(root, "gone")
	state.Workspaces[0] = workspace
	got, err = EnrichWorktrees(state, repoID, KindGit, []Worktree{{Path: path, VCS: KindGit}})
	require.NoError(t, err)
	require.Empty(t, got[0].WorkspaceID)
}

func TestIdentityJSONGoldenCompatibility(t *testing.T) {
	worktree := Worktree{Path: "/tmp/feature", Branch: "origin/feature", Head: "abc", VCS: KindGit,
		RepositoryID: "550e8400-e29b-41d4-a716-446655440001", WorkspaceID: "550e8400-e29b-41d4-a716-446655440000",
		Name: "repo/feature", NativeName: "repo/feature"}
	data, err := json.MarshalIndent(worktree, "", "  ")
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"/tmp/feature","repository_id":"550e8400-e29b-41d4-a716-446655440001","workspace_id":"550e8400-e29b-41d4-a716-446655440000","name":"repo/feature","native_name":"repo/feature","branch":"origin/feature","head":"abc","is_main":false,"is_bare":false,"is_detached":false,"prunable":false,"vcs":"git"}`, string(data))
}

func TestIDSelectionIsExact(t *testing.T) {
	wts := []Worktree{{WorkspaceID: "one", RepositoryID: "repo"}, {WorkspaceID: "two", RepositoryID: "repo"}}
	got, err := FindWorkspaceByID(wts, "one")
	require.NoError(t, err)
	require.Equal(t, "one", got.WorkspaceID)
	_, err = FindWorkspaceByID(wts, "o")
	require.ErrorIs(t, err, ErrWorktreeNotFound)
}
