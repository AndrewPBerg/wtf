package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type identityTestManager struct {
	kind                       vcs.Kind
	addCalls                   int
	addedRef                   string
	addErr, removeErr, initErr error
	mainPath                   string
	stateDir                   string
}

func (m *identityTestManager) Kind() vcs.Kind                      { return m.kind }
func (m *identityTestManager) List(string) ([]vcs.Worktree, error) { return nil, nil }
func (m *identityTestManager) MainWorktree(string) (vcs.Worktree, error) {
	return vcs.Worktree{Path: m.mainPath, IsMain: true}, nil
}
func (m *identityTestManager) Find(string, string) (vcs.Worktree, error) { return vcs.Worktree{}, nil }
func (m *identityTestManager) Add(_, ref, _ string) (string, error) {
	m.addCalls++
	m.addedRef = ref
	if m.addErr != nil {
		return "", m.addErr
	}
	return vcs.WorktreePath(m.mainPath, ref), nil
}
func (m *identityTestManager) Remove(_, _, _ string, _ bool) error      { return m.removeErr }
func (m *identityTestManager) RemoteURL(string) (string, error)         { return "", nil }
func (m *identityTestManager) StateDir(string) (string, error)          { return m.stateDir, nil }
func (m *identityTestManager) CurrentRef(string) (string, error)        { return "", nil }
func (m *identityTestManager) FetchRefspec(_, _, _ string) error        { return nil }
func (m *identityTestManager) Cleanable(string) ([]vcs.Worktree, error) { return nil, nil }
func (m *identityTestManager) InitGitDiff(string) error                 { return m.initErr }

type identityTestLifecycle struct {
	pending                                        identity.Workspace
	createErr                                      error
	activateCalls, removeCalls, cleanupFailedCalls int
	removeErr                                      error
}

func (l *identityTestLifecycle) CreateWorkspace(repo, name, backend, native, path string) (identity.Workspace, error) {
	if l.createErr != nil {
		return identity.Workspace{}, l.createErr
	}
	l.pending = identity.Workspace{ID: "11111111-1111-4111-8111-111111111111", RepositoryID: repo, Name: name, Backend: backend, NativeName: native, Path: path, LifecycleState: identity.Pending}
	return l.pending, nil
}
func (l *identityTestLifecycle) ActivateWorkspace(string) (identity.Workspace, error) {
	l.activateCalls++
	l.pending.LifecycleState = identity.Active
	return l.pending, nil
}
func (l *identityTestLifecycle) MoveWorkspace(_, path string) (identity.Workspace, error) {
	l.pending.Path = path
	return l.pending, nil
}
func (l *identityTestLifecycle) RemoveWorkspace(string) (identity.Workspace, error) {
	l.removeCalls++
	if l.removeErr != nil {
		return identity.Workspace{}, l.removeErr
	}
	l.pending.LifecycleState = identity.Removed
	return l.pending, nil
}
func (l *identityTestLifecycle) MarkCleanupFailed(string) (identity.Workspace, error) {
	l.cleanupFailedCalls++
	l.pending.LifecycleState = identity.CleanupFailed
	return l.pending, nil
}

type identityTestRepoResolver struct{}

func (identityTestRepoResolver) ResolveRepository(string, string) (identity.Repository, error) {
	return identity.Repository{ID: "22222222-2222-4222-8222-222222222222", LifecycleState: identity.Active}, nil
}
func identityTestCmd() *cobra.Command { c, _, _ := newTestCmd(""); return c }

func withIdentityFakes(t *testing.T, l *identityTestLifecycle) {
	t.Helper()
	old := identityDependencies
	identityDependencies = func() (repositoryResolver, identityLifecycle, error) { return identityTestRepoResolver{}, l, nil }
	t.Cleanup(func() { identityDependencies = old })
}

func TestCanonicalWorkspaceName(t *testing.T) {
	tests := []struct {
		name, repo, requested, want string
	}{
		{"unscoped", "MyRepo", "feature/auth", "myrepo/feature/auth"},
		{"already scoped", "myrepo", "myrepo/feature/auth", "myrepo/feature/auth"},
		{"nested unscoped", "myrepo", "feature/frontend/login", "myrepo/feature/frontend/login"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalWorkspaceName(tt.repo, tt.requested)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCLIFixturesIgnoreExternalWTFHome(t *testing.T) {
	external := t.TempDir()
	t.Setenv("WTF_HOME", external)
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := createIdentityWorkspace(identityTestCmd(), wm, nil, dir, "fixture", "main")
	require.NoError(t, err)

	for _, name := range []string{"state.json", "state.lock", "repos.json"} {
		assert.NoFileExists(t, filepath.Join(external, name), "fixture wrote into inherited WTF_HOME")
	}
}

func TestCLIIdentityUsesOnlySuppliedWTFHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)
	outside := t.TempDir()
	mainPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(mainPath, 0o755))
	m := &identityTestManager{kind: vcs.KindGit, mainPath: mainPath, stateDir: t.TempDir()}
	old := identityDependencies
	identityDependencies = defaultIdentityDependencies
	t.Cleanup(func() { identityDependencies = old })
	_, err := createIdentityWorkspace(identityTestCmd(), m, nil, mainPath, "feature", "main")
	require.NoError(t, err)
	store, err := identity.NewStore(home)
	require.NoError(t, err)
	statePath, _ := store.Paths()
	assert.FileExists(t, statePath)
	other, err := identity.NewStore(outside)
	require.NoError(t, err)
	otherState, _ := other.Paths()
	assert.NoFileExists(t, otherState)
}

func TestCreateIdentityWorkspaceRejectsDuplicateBeforeAdd(t *testing.T) {
	l := &identityTestLifecycle{createErr: errors.New("duplicate")}
	withIdentityFakes(t, l)
	m := &identityTestManager{kind: vcs.KindGit, mainPath: t.TempDir()}
	_, err := createIdentityWorkspace(identityTestCmd(), m, nil, m.mainPath, "feature", "main")
	require.Error(t, err)
	assert.Zero(t, m.addCalls)
}
func TestCreateIdentityWorkspaceActivatesExactlyOnce(t *testing.T) {
	l := &identityTestLifecycle{}
	withIdentityFakes(t, l)
	m := &identityTestManager{kind: vcs.KindGit, mainPath: t.TempDir()}
	_, err := createIdentityWorkspace(identityTestCmd(), m, nil, m.mainPath, "feature", "main")
	require.NoError(t, err)
	assert.Equal(t, 1, l.activateCalls)
	assert.Equal(t, "feature", m.addedRef)
}
func TestCreateIdentityWorkspaceVCSFailureRemovesPending(t *testing.T) {
	l := &identityTestLifecycle{}
	withIdentityFakes(t, l)
	m := &identityTestManager{kind: vcs.KindGit, mainPath: t.TempDir(), addErr: errors.New("vcs")}
	_, err := createIdentityWorkspace(identityTestCmd(), m, nil, m.mainPath, "feature", "main")
	require.Error(t, err)
	assert.Equal(t, 1, l.removeCalls)
	assert.Zero(t, l.activateCalls)
}
func TestCreateIdentityWorkspaceRollbackFailureRetainsCleanupFailed(t *testing.T) {
	l := &identityTestLifecycle{}
	withIdentityFakes(t, l)
	m := &identityTestManager{kind: vcs.KindJJ, mainPath: t.TempDir(), initErr: errors.New("setup"), removeErr: errors.New("rollback")}
	_, err := createIdentityWorkspace(identityTestCmd(), m, nil, m.mainPath, "feature", "main")
	require.Error(t, err)
	assert.Equal(t, 1, l.cleanupFailedCalls)
	assert.Zero(t, l.activateCalls)
}
