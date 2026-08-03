package jj

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// requireJJ skips the test when the jj CLI is unavailable, so the suite still
// runs on machines (and CI images) without jj installed.
func requireJJ(t *testing.T) {
	t.Helper()
	if _, err := osexec.LookPath("jj"); err != nil {
		t.Skip("jj not installed — skipping jj integration test")
	}
}

// newTestRepo creates a colocated jj repo with one commit and a `main` bookmark,
// returning its root. The repo lives in a temp dir cleaned up by the test.
func newTestRepo(t *testing.T) string {
	t.Helper()
	requireJJ(t)

	// The repo is nested one level down so sibling workspace paths (which land
	// beside the repo) also live inside the temp dir.
	base := t.TempDir()
	// Resolve symlinks up front: on macOS t.TempDir() sits under /var, a symlink
	// to /private/var, and jj reports the resolved form.
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	root := filepath.Join(base, "myrepo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	runJJ(t, root, "git", "init", "--colocate")
	// jj refuses to create commits without an identity configured.
	runJJ(t, root, "config", "set", "--repo", "user.name", "wtf test")
	runJJ(t, root, "config", "set", "--repo", "user.email", "wtf@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".env\n"), 0o644))
	runJJ(t, root, "commit", "-m", "init")
	runJJ(t, root, "bookmark", "create", "main", "-r", "@-")

	return root
}

// runJJ runs a jj command in dir, failing the test on error.
func runJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := (&RealExecutor{}).Run(dir, args...)
	require.NoError(t, err, "jj %v", args)
	return out
}

// runPlainGit runs git in dir without an explicit --git-dir, for building upstream
// fixtures that jj then clones.
func runPlainGit(dir string, args ...string) error {
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}
