package jj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func TestGitDiffShadowShowsJJWorkspaceChanges(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)

	require.NoError(t, m.InitGitDiff(wsPath))
	assert.FileExists(t, filepath.Join(wsPath, ".git", vcs.JJGitDiffMarker))

	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Empty(t, status)

	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("changed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "new.txt"), []byte("new\n"), 0o644))

	status, err = runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Contains(t, status, "M a.txt")
	assert.Contains(t, status, "?? new.txt")
	assert.NotContains(t, status, ".jj")

	jjStatus := runJJ(t, wsPath, "status")
	assert.Contains(t, jjStatus, "M a.txt")
	assert.Contains(t, jjStatus, "A new.txt")
}

func TestRefreshGitDiffClearsGitStagingWithoutChangingFiles(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)
	require.NoError(t, m.InitGitDiff(wsPath))

	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("changed\n"), 0o644))
	_, err = runWorkspaceGit(wsPath, "add", "a.txt")
	require.NoError(t, err)
	require.NoError(t, m.RefreshGitDiff(wsPath))

	cached, err := runWorkspaceGit(wsPath, "diff", "--cached", "--name-only")
	require.NoError(t, err)
	assert.Empty(t, cached)
	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Contains(t, status, "M a.txt", "the working-copy edit must remain visible")
	content, err := os.ReadFile(filepath.Join(wsPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "changed\n", string(content))
}

func TestRefreshGitDiffTracksNewJJParentWithoutChangingFiles(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)
	require.NoError(t, m.InitGitDiff(wsPath))

	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("changed\n"), 0o644))
	runJJ(t, wsPath, "new") // The edited change becomes @-; the new @ is empty.

	before, err := os.ReadFile(filepath.Join(wsPath, "a.txt"))
	require.NoError(t, err)
	require.NoError(t, m.RefreshGitDiff(wsPath))
	after, err := os.ReadFile(filepath.Join(wsPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, before, after, "refresh must never update working-copy files")

	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Empty(t, status, "Git baseline should now match the new jj parent")
}

func TestRefreshGitDiffRepairsAlternatesAfterRepoMove(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)
	require.NoError(t, m.InitGitDiff(wsPath))

	oldBase := filepath.Dir(root)
	newBase := oldBase + "-moved"
	require.NoError(t, os.Rename(oldBase, newBase))
	t.Cleanup(func() { _ = os.RemoveAll(newBase) })
	movedRoot := filepath.Join(newBase, filepath.Base(root))
	movedWS := filepath.Join(newBase, filepath.Base(wsPath))

	require.NoError(t, m.RefreshGitDiff(movedWS))
	alternates, err := os.ReadFile(filepath.Join(movedWS, ".git", "objects", "info", "alternates"))
	require.NoError(t, err)
	assert.Contains(t, string(alternates), filepath.Join(movedRoot, ".git", "objects"))
	status, err := runWorkspaceGit(movedWS, "status", "--short")
	require.NoError(t, err)
	assert.Empty(t, status)
}

func TestGitDiffShadowWorksForNonColocatedRepo(t *testing.T) {
	root := newTestRepoWithRemote(t, false)
	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)

	require.NoError(t, m.InitGitDiff(wsPath))
	alternates, err := os.ReadFile(filepath.Join(wsPath, ".git", "objects", "info", "alternates"))
	require.NoError(t, err)
	assert.Contains(t, string(alternates), filepath.Join(root, ".jj", "repo", "store", "git", "objects"))

	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Empty(t, status)
}

func TestGitDiffShadowSupportsVirtualRootParent(t *testing.T) {
	requireJJ(t)
	base := t.TempDir()
	root := filepath.Join(base, "empty")
	require.NoError(t, os.MkdirAll(root, 0o755))
	runJJ(t, root, "git", "init", "--colocate")

	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "")
	require.NoError(t, err)
	require.NoError(t, m.InitGitDiff(wsPath))

	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "new.txt"), []byte("new\n"), 0o644))
	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Equal(t, "?? new.txt", status)
}

func TestGitDiffShadowMatchesSHA256BackingRepo(t *testing.T) {
	requireJJ(t)
	base := t.TempDir()
	root := filepath.Join(base, "sha256")
	require.NoError(t, os.MkdirAll(root, 0o755))
	runJJ(t, root, "git", "init", "--colocate", "--object-hash", "sha256")
	runJJ(t, root, "config", "set", "--repo", "user.name", "wtf test")
	runJJ(t, root, "config", "set", "--repo", "user.email", "wtf@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi\n"), 0o644))
	runJJ(t, root, "commit", "-m", "init")
	runJJ(t, root, "bookmark", "create", "main", "-r", "@-")

	m := NewWorkspaceManager(&RealExecutor{})
	wsPath, err := m.Add(root, "zed", "main")
	require.NoError(t, err)
	require.NoError(t, m.InitGitDiff(wsPath))

	format, err := runWorkspaceGit(wsPath, "rev-parse", "--show-object-format")
	require.NoError(t, err)
	assert.Equal(t, "sha256", format)
	status, err := runWorkspaceGit(wsPath, "status", "--short")
	require.NoError(t, err)
	assert.Empty(t, status)
}

type fixedJJExecutor struct {
	output string
}

func (e fixedJJExecutor) Run(_ string, _ ...string) (string, error) { return e.output, nil }

func TestGitDiffBaseRejectsMultipleParents(t *testing.T) {
	m := NewWorkspaceManager(fixedJJExecutor{output: "abc\ndef\n"})
	_, err := m.gitDiffBase("/unused")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one jj parent; found 2")
}

func TestGitDiffShadowRefusesExistingGitMetadata(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	err := m.InitGitDiff(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already contains .git")
}
