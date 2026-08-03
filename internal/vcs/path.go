package vcs

import (
	"os"
	"path/filepath"
	"strings"
)

// WorktreePath computes the sibling checkout directory path. The ref name is the
// prefix so checkouts sort by purpose:
//
//	/code/myrepo + feature/auth → /code/feature-auth--myrepo
//	/code/myrepo + pr-711       → /code/pr-711--myrepo
//
// Both backends use this, so a git worktree and a jj workspace for the same ref
// land at the same place.
func WorktreePath(mainPath, ref string) string {
	sanitized := strings.ReplaceAll(ref, "/", "-")
	parent := filepath.Dir(mainPath)
	base := filepath.Base(mainPath)
	return filepath.Join(parent, sanitized+"--"+base)
}

// IsInside reports whether cwd is equal to or nested under root.
func IsInside(cwd, root string) bool {
	cwd = filepath.Clean(cwd)
	root = filepath.Clean(root)
	if cwd == root {
		return true
	}
	return strings.HasPrefix(cwd, root+string(filepath.Separator))
}

// repoLocationEnv are the environment variables git consults to locate a
// repository, overriding the working directory it was invoked in.
//
// git sets these for every hook it runs, so anything invoking git from inside a
// hook inherits them. wtf always names the repo it means — via the command's
// working directory, or an explicit --git-dir — so inheriting these can only break
// that intent, pointing git at the wrong repository or a stale index.
var repoLocationEnv = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_PREFIX",
}

// SanitizedEnv returns the current environment with git's repo-location variables
// removed. Everything else is preserved, so credential helpers, GIT_SSH_COMMAND,
// and proxy settings keep working.
func SanitizedEnv() []string {
	drop := make(map[string]bool, len(repoLocationEnv))
	for _, k := range repoLocationEnv {
		drop[k] = true
	}

	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if ok && drop[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}
