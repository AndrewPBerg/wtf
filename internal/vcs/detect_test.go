package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkGit marks dir as a git checkout. asFile mimics a git worktree, where .git is
// a file pointing at the real git dir.
func mkGit(t *testing.T, dir string, asFile bool) {
	t.Helper()
	p := filepath.Join(dir, ".git")
	if asFile {
		require.NoError(t, os.WriteFile(p, []byte("gitdir: /elsewhere"), 0o644))
		return
	}
	require.NoError(t, os.MkdirAll(p, 0o755))
}

// mkJJ marks dir as a jj workspace. asMain creates .jj/repo as a directory (the
// primary workspace) rather than a file pointer (a secondary one).
func mkJJ(t *testing.T, dir string, asMain bool) {
	t.Helper()
	if asMain {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".jj", "repo"), 0o755))
		return
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".jj"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".jj", "repo"),
		[]byte("../../main/.jj/repo"), 0o644))
}

func TestDetectionHelpers(t *testing.T) {
	both := Detection{Root: "/r", Kinds: []Kind{KindGit, KindJJ}}
	gitOnly := Detection{Root: "/r", Kinds: []Kind{KindGit}}
	jjOnly := Detection{Root: "/r", Kinds: []Kind{KindJJ}}
	none := Detection{}

	assert.True(t, both.Has(KindGit))
	assert.True(t, both.Has(KindJJ))
	assert.True(t, both.Colocated())

	assert.True(t, gitOnly.Has(KindGit))
	assert.False(t, gitOnly.Has(KindJJ))
	assert.False(t, gitOnly.Colocated())

	assert.True(t, jjOnly.Has(KindJJ))
	assert.False(t, jjOnly.Has(KindGit))
	assert.False(t, jjOnly.Colocated())

	assert.False(t, none.Has(KindGit))
	assert.False(t, none.Colocated())
}

func TestDetect(t *testing.T) {
	t.Run("git only", func(t *testing.T) {
		dir := t.TempDir()
		mkGit(t, dir, false)

		det, err := Detect(dir)
		require.NoError(t, err)
		assert.Equal(t, dir, det.Root)
		assert.Equal(t, []Kind{KindGit}, det.Kinds)
	})

	t.Run("jj only", func(t *testing.T) {
		dir := t.TempDir()
		mkJJ(t, dir, true)

		det, err := Detect(dir)
		require.NoError(t, err)
		assert.Equal(t, []Kind{KindJJ}, det.Kinds)
	})

	t.Run("colocated reports both, git first", func(t *testing.T) {
		dir := t.TempDir()
		mkGit(t, dir, false)
		mkJJ(t, dir, true)

		det, err := Detect(dir)
		require.NoError(t, err)
		assert.Equal(t, []Kind{KindGit, KindJJ}, det.Kinds)
		assert.True(t, det.Colocated())
	})

	t.Run("git worktree with .git as a file still counts", func(t *testing.T) {
		dir := t.TempDir()
		mkGit(t, dir, true)

		det, err := Detect(dir)
		require.NoError(t, err)
		assert.Equal(t, []Kind{KindGit}, det.Kinds)
	})

	t.Run("walks up from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		mkGit(t, root, false)
		sub := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(sub, 0o755))

		det, err := Detect(sub)
		require.NoError(t, err)
		assert.Equal(t, root, det.Root)
	})

	t.Run("a jj workspace under a git repo is not colocated", func(t *testing.T) {
		// This is the case that makes stopping at the first match essential: a
		// secondary jj workspace has no .git of its own, and continuing upward
		// would find an unrelated ancestor git repo and wrongly call it colocated.
		outer := t.TempDir()
		mkGit(t, outer, false)

		ws := filepath.Join(outer, "nested-workspace")
		require.NoError(t, os.MkdirAll(ws, 0o755))
		mkJJ(t, ws, false)

		det, err := Detect(ws)
		require.NoError(t, err)
		assert.Equal(t, ws, det.Root)
		assert.Equal(t, []Kind{KindJJ}, det.Kinds)
		assert.False(t, det.Colocated(), "the ancestor git repo must not leak in")
	})

	t.Run("not a repo", func(t *testing.T) {
		_, err := Detect(t.TempDir())
		assert.ErrorIs(t, err, ErrNotARepo)
	})

	t.Run(".jj as a file is not a jj repo", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".jj"), []byte("x"), 0o644))

		_, err := Detect(dir)
		assert.ErrorIs(t, err, ErrNotARepo)
	})
}

func TestAvailable(t *testing.T) {
	prev := binaryAvailable
	t.Cleanup(func() { binaryAvailable = prev })

	var asked []string
	binaryAvailable = func(name string) bool {
		asked = append(asked, name)
		return name == "git"
	}

	assert.True(t, Available(KindGit))
	assert.False(t, Available(KindJJ))
	assert.False(t, Available(Kind("hg")), "an unknown kind has no binary to look for")
	assert.Equal(t, []string{"git", "jj"}, asked)
}

func TestAvailableUsesRealLookup(t *testing.T) {
	// git is a hard dependency of the test suite itself, so it must be found by
	// the real implementation.
	assert.True(t, Available(KindGit))
}
