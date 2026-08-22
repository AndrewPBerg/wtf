package jj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func TestParseWorkspaceList(t *testing.T) {
	const sep = fieldSep

	tests := []struct {
		name     string
		output   string
		mainRoot string
		want     []vcs.Worktree
	}{
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:     "single main workspace",
			output:   "default" + sep + "/code/repo" + sep + "abc123def456" + sep + "wkvwxyrysvll" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{
					Branch: "default", Path: "/code/repo", Head: "abc123def456",
					ChangeID: "wkvwxyrysvll", IsMain: true, VCS: vcs.KindJJ,
				},
			},
		},
		{
			name: "main is hoisted above alphabetically earlier workspaces",
			output: "aaa" + sep + "/code/aaa--repo" + sep + "111111111111" + sep + "cccccccccccc" + sep + "\n" +
				"default" + sep + "/code/repo" + sep + "222222222222" + sep + "dddddddddddd" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{
					Branch: "default", Path: "/code/repo", Head: "222222222222",
					ChangeID: "dddddddddddd", IsMain: true, VCS: vcs.KindJJ,
				},
				{
					Branch: "aaa", Path: "/code/aaa--repo", Head: "111111111111",
					ChangeID: "cccccccccccc", VCS: vcs.KindJJ,
				},
			},
		},
		{
			name:     "bookmarks are split on comma",
			output:   "feat" + sep + "/code/feat--repo" + sep + "abc" + sep + "chg" + sep + "feat,origin-feat",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{
					Branch: "feat", Path: "/code/feat--repo", Head: "abc", ChangeID: "chg",
					Bookmarks: []string{"feat", "origin-feat"}, VCS: vcs.KindJJ,
				},
			},
		},
		{
			name:     "workspace with a deleted directory is prunable",
			output:   "gone" + sep + "<Error: Failed to resolve workspace root: gone>" + sep + "abc" + sep + "chg" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{Branch: "gone", Path: "", Head: "abc", ChangeID: "chg", Prunable: true, VCS: vcs.KindJJ},
			},
		},
		{
			name:     "jj 0.44 empty root is prunable",
			output:   "gone" + sep + sep + "abc" + sep + "chg" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{Branch: "gone", Path: "", Head: "abc", ChangeID: "chg", Prunable: true, VCS: vcs.KindJJ},
			},
		},
		{
			name:     "workspace name containing a slash survives round trip",
			output:   "feat/auth" + sep + "/code/feat-auth--repo" + sep + "abc" + sep + "chg" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{Branch: "feat/auth", Path: "/code/feat-auth--repo", Head: "abc", ChangeID: "chg", VCS: vcs.KindJJ},
			},
		},
		{
			name:     "malformed lines are skipped",
			output:   "broken-line-with-no-separators\n" + "ok" + sep + "/code/ok--repo" + sep + "abc" + sep + "chg" + sep + "",
			mainRoot: "/code/repo",
			want: []vcs.Worktree{
				{Branch: "ok", Path: "/code/ok--repo", Head: "abc", ChangeID: "chg", VCS: vcs.KindJJ},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWorkspaceList(tt.output, tt.mainRoot)
			sortMainFirst(got)
			// Branch is the pre-identity compatibility field; JJ exposes its
			// native name as the canonical workspace name too.
			for i := range tt.want {
				tt.want[i].Name = tt.want[i].Branch
				tt.want[i].NativeName = tt.want[i].Branch
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseWorkspaceListUsesCanonicalNameAsNativeJJName(t *testing.T) {
	wts := parseWorkspaceList("repo/feature"+fieldSep+"/code/feature--repo"+fieldSep+"abc"+fieldSep+"change"+fieldSep, "/code/repo")
	require.Len(t, wts, 1)
	assert.Equal(t, "repo/feature", wts[0].Name)
	assert.Equal(t, wts[0].Name, wts[0].NativeName)
	// Keep the old structured field stable for existing consumers.
	assert.Equal(t, wts[0].Name, wts[0].Branch)
}

func TestValidateRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{name: "simple name", ref: "feat"},
		{name: "slashes are allowed by jj", ref: "feature/auth"},
		{name: "dashes and digits", ref: "pr-711"},
		{name: "empty", ref: "", wantErr: true},
		{name: "whitespace only", ref: "   ", wantErr: true},
		{name: "embedded space", ref: "feat auth", wantErr: true},
		{name: "embedded tab", ref: "feat\tauth", wantErr: true},
		{name: "field separator", ref: "feat\x1fauth", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRef(tt.ref)
			if tt.wantErr {
				assert.ErrorIs(t, err, vcs.ErrInvalidRef)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestMainRootFromWorkspace(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	// From the main workspace.
	got, err := MainRoot(root)
	require.NoError(t, err)
	assert.Equal(t, root, got)

	// From a secondary workspace, which has a .jj/repo *file* pointer and no
	// .git of its own — the case that breaks plain git discovery.
	got, err = MainRoot(wsPath)
	require.NoError(t, err)
	assert.Equal(t, root, got)

	assert.NoDirExists(t, filepath.Join(wsPath, ".git"))
}

func TestAddListFindRemove(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat/auth", "main")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(root), "feat-auth--myrepo"), wsPath)
	assert.DirExists(t, wsPath)

	wts, err := m.List(root)
	require.NoError(t, err)
	require.Len(t, wts, 2)
	assert.True(t, wts[0].IsMain, "main workspace must sort first")
	assert.Equal(t, vcs.KindJJ, wts[0].VCS)

	// Find by exact workspace name.
	found, err := m.Find(root, "feat/auth")
	require.NoError(t, err)
	assert.Equal(t, wsPath, found.Path)
	assert.False(t, found.IsMain)

	// Find by substring.
	found, err = m.Find(root, "auth")
	require.NoError(t, err)
	assert.Equal(t, "feat/auth", found.Branch)

	// CurrentRef resolves the workspace name from its own directory.
	ref, err := m.CurrentRef(wsPath)
	require.NoError(t, err)
	assert.Equal(t, "feat/auth", ref)

	require.NoError(t, m.Remove(root, "feat/auth", root, false))
	assert.NoDirExists(t, wsPath, "Remove must delete the directory jj leaves behind")

	wts, err = m.List(root)
	require.NoError(t, err)
	assert.Len(t, wts, 1)
}

func TestRemoveRefusesMainAndCurrentDir(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	mainWt, err := m.MainWorktree(root)
	require.NoError(t, err)
	assert.ErrorIs(t, m.Remove(root, mainWt.Branch, root, false), vcs.ErrMainWorktree)

	// Refuse to remove the workspace the caller is standing in.
	err = m.Remove(root, "feat", filepath.Join(wsPath, "sub"), false)
	assert.ErrorIs(t, err, vcs.ErrWorktreeIsCurrentDir)
	assert.DirExists(t, wsPath)
}

func TestRemoveDetectsUncommittedChanges(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	// Dirty the workspace's working copy.
	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("changed\n"), 0o644))

	err = m.Remove(root, "feat", root, false)
	assert.ErrorIs(t, err, vcs.ErrWorktreeHasChanges)
	assert.DirExists(t, wsPath)

	// --force overrides.
	require.NoError(t, m.Remove(root, "feat", root, true))
	assert.NoDirExists(t, wsPath)
}

func TestAddDuplicateNameIsReported(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	_, err = m.Add(root, "feat", "main")
	assert.ErrorIs(t, err, vcs.ErrBranchAlreadyInUse)
}

func TestAddIgnoredFilesAreNotCopied(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	// .env is gitignored in the fixture, so jj will not carry it across. This is
	// exactly why wtf's env setup has to run for jj workspaces too.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1\n"), 0o644))

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(wsPath, "a.txt"))
	assert.NoFileExists(t, filepath.Join(wsPath, ".env"))
}

func TestStateDirIsSharedAcrossWorkspaces(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	fromMain, err := m.StateDir(root)
	require.NoError(t, err)
	fromWs, err := m.StateDir(wsPath)
	require.NoError(t, err)

	assert.Equal(t, fromMain, fromWs, "every workspace must agree on one state dir")
	assert.Equal(t, filepath.Join(root, ".jj", "repo", "wtf"), fromMain)
}

func TestListMarksPrunableWorkspace(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(wsPath))

	wts, err := m.List(root)
	require.NoError(t, err)

	var found bool
	for _, wt := range wts {
		if wt.Branch == "feat" {
			found = true
			assert.True(t, wt.Prunable, "workspace with a deleted directory should be prunable")
		}
	}
	assert.True(t, found, "forgotten-but-registered workspace should still be listed")
}

func TestRemoteURL(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.RemoteURL(root)
	assert.Error(t, err, "no remote configured yet")

	runJJ(t, root, "git", "remote", "add", "origin", "git@github.com:foo/bar.git")

	url, err := m.RemoteURL(root)
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:foo/bar.git", url)
}

func TestAddWithEmptyBaseProducesPopulatedWorkspace(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	// An empty base means "wherever the main line is". The fixture repo has no
	// remote, where trunk() resolves to the *root* commit — whose tree is empty.
	// Basing a workspace there silently yields an empty directory, so the backend
	// must fall through to jj's own default instead.
	wsPath, err := m.Add(root, "feat", "")
	require.NoError(t, err)
	require.DirExists(t, wsPath)

	assert.FileExists(t, filepath.Join(wsPath, "a.txt"),
		"workspace must contain the project files, not an empty root tree")
}

func TestRevsetUsableRejectsRootCommit(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	assert.False(t, m.revsetUsable(root, "root()"),
		"the root commit has an empty tree and is never a usable base")
	assert.True(t, m.revsetUsable(root, "main"))
	assert.False(t, m.revsetUsable(root, "nonexistent-bookmark"))
}

func TestWorkspaceManagerKind(t *testing.T) {
	assert.Equal(t, vcs.KindJJ, NewWorkspaceManager(&RealExecutor{}).Kind())
}

func TestWorkspaceRootErrors(t *testing.T) {
	_, err := WorkspaceRoot(t.TempDir())
	assert.ErrorIs(t, err, vcs.ErrNotARepo)

	_, err = MainRoot(t.TempDir())
	assert.ErrorIs(t, err, vcs.ErrNotARepo)
}

func TestWorkspaceRootFromSubdirectory(t *testing.T) {
	root := newTestRepo(t)
	sub := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	got, err := WorkspaceRoot(sub)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestFindErrors(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.Add(root, "feat-one", "main")
	require.NoError(t, err)
	_, err = m.Add(root, "feat-two", "main")
	require.NoError(t, err)

	_, err = m.Find(root, "nothing-like-this")
	assert.ErrorIs(t, err, vcs.ErrWorktreeNotFound)

	// "feat-" matches both, which is ambiguous.
	_, err = m.Find(root, "feat-")
	assert.ErrorIs(t, err, vcs.ErrMultipleMatches)
}

func TestFindByBookmark(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)
	runJJ(t, wsPath, "bookmark", "create", "shipit", "-r", "@")

	// A workspace is reachable by a bookmark pointing at its working copy, even
	// though wtf never creates bookmarks itself.
	found, err := m.Find(root, "shipit")
	require.NoError(t, err)
	assert.Equal(t, "feat", found.Branch)
	assert.Contains(t, found.Bookmarks, "shipit")
}

func TestCurrentRefUnknownDirectory(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.CurrentRef(filepath.Dir(root))
	assert.Error(t, err, "a directory that is not a workspace root has no workspace name")
}

func TestAddRejectsInvalidRef(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.Add(root, "bad name", "main")
	assert.ErrorIs(t, err, vcs.ErrInvalidRef)
}

func TestAddUnresolvableBaseIsReported(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	_, err := m.Add(root, "feat", "no-such-bookmark")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-bookmark")
}

func TestRemoveUnknownWorkspace(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	err := m.Remove(root, "never-existed", root, false)
	assert.ErrorIs(t, err, vcs.ErrWorktreeNotFound)
}

func TestRevsetResolves(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	assert.True(t, m.revsetResolves(root, "main"))
	assert.True(t, m.revsetResolves(root, "root()"),
		"the root commit resolves even though it is not a usable base")
	assert.False(t, m.revsetResolves(root, "no-such-bookmark"))
}

func TestCleanableSkipsWorkspacesWithRealWork(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "busy", "main")
	require.NoError(t, err)

	// A workspace holding actual changes must never be offered up for cleaning.
	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("work\n"), 0o644))
	runJJ(t, wsPath, "status")

	got, err := m.Cleanable(root)
	require.NoError(t, err)
	for _, wt := range got {
		assert.NotEqual(t, "busy", wt.Branch, "a dirty workspace is not cleanable")
	}
}

func TestCleanableReportsPrunable(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "gone", "main")
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(wsPath))

	got, err := m.Cleanable(root)
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "gone", got[0].Branch)
	assert.True(t, got[0].Prunable)
}

func TestCleanableNeverIncludesMain(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	got, err := m.Cleanable(root)
	require.NoError(t, err)
	for _, wt := range got {
		assert.False(t, wt.IsMain)
	}
}

func TestCleanableError(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	_, err := m.Cleanable(t.TempDir())
	assert.Error(t, err)
}

func TestListError(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	_, err := m.List(t.TempDir())
	assert.Error(t, err)
}

func TestStateDirError(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	_, err := m.StateDir(t.TempDir())
	assert.Error(t, err)
}

func TestExecutorReportsStderr(t *testing.T) {
	requireJJ(t)
	_, err := (&RealExecutor{}).Run(t.TempDir(), "workspace", "list")
	require.Error(t, err)
	// The message should carry jj's own diagnostics, not just an exit code.
	assert.Contains(t, err.Error(), "jj workspace list")
}

func TestMainWorktreeWhenMainCannotBeIdentified(t *testing.T) {
	// parseWorkspaceList with no mainRoot marks nothing as main, which is the
	// state MainWorktree has to report rather than silently returning entry zero.
	wts := parseWorkspaceList(
		"a"+fieldSep+"/code/a"+fieldSep+"c1"+fieldSep+"ch1"+fieldSep+"", "")
	require.Len(t, wts, 1)
	assert.False(t, wts[0].IsMain)
}

func TestMainWorktreeError(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	_, err := m.MainWorktree(t.TempDir())
	assert.Error(t, err)
}

func TestHasChangesRunsInsideTargetWorkspace(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "feat", "main")
	require.NoError(t, err)

	clean, err := m.hasChanges(wsPath)
	require.NoError(t, err)
	assert.False(t, clean)

	// jj only snapshots the working copy it is invoked from, so the check must
	// run with the target workspace as its working directory.
	require.NoError(t, os.WriteFile(filepath.Join(wsPath, "a.txt"), []byte("edit\n"), 0o644))

	dirty, err := m.hasChanges(wsPath)
	require.NoError(t, err)
	assert.True(t, dirty)

	_, err = m.hasChanges(t.TempDir())
	assert.Error(t, err)
}

func TestRemoveForgetsPrunableWorkspaceWithNoPath(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	wsPath, err := m.Add(root, "gone", "main")
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(wsPath))

	// The path is unrecoverable once the directory is gone, but the registration
	// must still be cleaned up.
	require.NoError(t, m.Remove(root, "gone", root, false))

	wts, err := m.List(root)
	require.NoError(t, err)
	for _, wt := range wts {
		assert.NotEqual(t, "gone", wt.Branch)
	}
}

func TestRemoteURLIgnoresNonOriginRemotes(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	runJJ(t, root, "git", "remote", "add", "upstream", "git@github.com:up/stream.git")
	_, err := m.RemoteURL(root)
	assert.Error(t, err, "only origin is used for forge integration")

	runJJ(t, root, "git", "remote", "add", "origin", "git@github.com:me/mine.git")
	url, err := m.RemoteURL(root)
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:me/mine.git", url)
}

func TestRemoteURLError(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	_, err := m.RemoteURL(t.TempDir())
	assert.Error(t, err)
}

func TestResolveBaseHonoursExplicitBase(t *testing.T) {
	root := newTestRepo(t)
	m := NewWorkspaceManager(&RealExecutor{})

	assert.Equal(t, "main", m.resolveBase(root, "main"))
	// No remote means trunk() is the root commit, so it must be declined.
	assert.Empty(t, m.resolveBase(root, ""))
}

// newTestRepoWithRemote creates an upstream git repo holding a `remote-feature`
// branch plus a PR-style ref, and clones it with jj, returning the clone's root.
// colocate controls whether the clone keeps a top-level .git.
func newTestRepoWithRemote(t *testing.T, colocate bool) string {
	t.Helper()
	requireJJ(t)

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	upstream := filepath.Join(base, "up")
	require.NoError(t, os.MkdirAll(upstream, 0o755))
	require.NoError(t, runPlainGit(upstream, "init", "-b", "main"))
	require.NoError(t, runPlainGit(upstream, "config", "user.email", "t@e.com"))
	require.NoError(t, runPlainGit(upstream, "config", "user.name", "t"))
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "f.txt"), []byte("hi\n"), 0o644))
	require.NoError(t, runPlainGit(upstream, "add", "-A"))
	require.NoError(t, runPlainGit(upstream, "commit", "-m", "init"))
	require.NoError(t, runPlainGit(upstream, "checkout", "-b", "remote-feature"))
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "f.txt"), []byte("feat\n"), 0o644))
	require.NoError(t, runPlainGit(upstream, "commit", "-am", "feat"))
	// A ref outside refs/heads, standing in for a forge PR ref.
	require.NoError(t, runPlainGit(upstream, "update-ref", "refs/pull/42/head", "remote-feature"))
	require.NoError(t, runPlainGit(upstream, "checkout", "main"))

	repo := filepath.Join(base, "clone")
	args := []string{"git", "clone"}
	if !colocate {
		args = append(args, "--no-colocate")
	}
	args = append(args, upstream, repo)
	_, err := (&RealExecutor{}).Run(base, args...)
	require.NoError(t, err)

	_, err = (&RealExecutor{}).Run(repo, "config", "set", "--repo", "user.name", "t")
	require.NoError(t, err)
	_, err = (&RealExecutor{}).Run(repo, "config", "set", "--repo", "user.email", "t@e.com")
	require.NoError(t, err)

	return repo
}

func TestGitDirResolution(t *testing.T) {
	t.Run("colocated points at the top-level .git", func(t *testing.T) {
		repo := newTestRepoWithRemote(t, true)
		gd, err := GitDir(repo)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(repo, ".git"), gd)
	})

	t.Run("non-colocated points inside .jj", func(t *testing.T) {
		repo := newTestRepoWithRemote(t, false)
		gd, err := GitDir(repo)
		require.NoError(t, err)
		// This is exactly why fetching cannot just shell out to `git`: the repo
		// git needs is not discoverable from the working directory.
		assert.Equal(t, filepath.Join(repo, ".jj", "repo", "store", "git"), gd)
		assert.NoDirExists(t, filepath.Join(repo, ".git"))
	})

	t.Run("not a jj repo", func(t *testing.T) {
		_, err := GitDir(t.TempDir())
		assert.Error(t, err)
	})
}

func TestFetchRefspecOnNonColocatedRepo(t *testing.T) {
	// The regression: `wtf new --branch` used to run plain `git fetch` here and die
	// with "not a git repository", because the git repo lives inside .jj.
	repo := newTestRepoWithRemote(t, false)
	m := NewWorkspaceManager(&RealExecutor{})

	require.NoError(t, m.FetchRefspec(repo, "origin", "remote-feature:remote-feature"))

	// The fetched ref must be usable as a base revision.
	assert.True(t, m.revsetUsable(repo, "remote-feature"))

	wsPath, err := m.Add(repo, "remote-feature", "remote-feature")
	require.NoError(t, err)
	require.DirExists(t, wsPath)

	content, err := os.ReadFile(filepath.Join(wsPath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "feat\n", string(content), "workspace must hold the fetched branch's content")
}

func TestFetchRefspecHandlesPRStyleRef(t *testing.T) {
	// jj git fetch cannot express a refspec like pull/42/head:pr-42, which is what
	// forge PR checkout needs — so this goes through the backing git repo.
	repo := newTestRepoWithRemote(t, false)
	m := NewWorkspaceManager(&RealExecutor{})

	require.NoError(t, m.FetchRefspec(repo, "origin", "pull/42/head:pr-42"))

	wsPath, err := m.Add(repo, "pr-42", "pr-42")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(wsPath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "feat\n", string(content))
}

func TestFetchRefspecOnColocatedRepo(t *testing.T) {
	repo := newTestRepoWithRemote(t, true)
	m := NewWorkspaceManager(&RealExecutor{})

	require.NoError(t, m.FetchRefspec(repo, "origin", "remote-feature:remote-feature"))
	assert.True(t, m.revsetUsable(repo, "remote-feature"))
}

func TestFetchRefspecErrors(t *testing.T) {
	m := NewWorkspaceManager(&RealExecutor{})
	assert.Error(t, m.FetchRefspec(t.TempDir(), "origin", "a:b"))

	repo := newTestRepoWithRemote(t, false)
	assert.Error(t, m.FetchRefspec(repo, "origin", "no-such-branch:no-such-branch"))
}

func TestCurrentOperationIDIsReadOnly(t *testing.T) {
	root := newTestRepo(t)
	manager := NewWorkspaceManager(&RealExecutor{})
	before := runJJ(t, root, "operation", "log", "--no-graph", "-n", "1", "-T", `id`)
	got, err := manager.CurrentOperationID(root)
	require.NoError(t, err)
	assert.Equal(t, before, got)
	after := runJJ(t, root, "operation", "log", "--no-graph", "-n", "1", "-T", `id`)
	assert.Equal(t, before, after)
}
