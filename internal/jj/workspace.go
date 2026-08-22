package jj

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// fieldSep separates template fields. jj templates emit \x1f as a literal byte,
// and it cannot occur in workspace names, paths, or bookmark names — so it is
// safe where a tab or space would be ambiguous.
const fieldSep = "\x1f"

// listTemplate renders one workspace per line as
// name ⟨sep⟩ root ⟨sep⟩ commit ⟨sep⟩ change ⟨sep⟩ bookmarks.
const listTemplate = `name ++ "\x1f" ++ self.root() ++ "\x1f" ++ ` +
	`target.commit_id().short(12) ++ "\x1f" ++ target.change_id().short(12) ++ "\x1f" ++ ` +
	`target.local_bookmarks().map(|b| b.name()).join(",") ++ "\n"`

// WorkspaceManager implements vcs.Manager for jj repositories.
type WorkspaceManager struct {
	executor Executor
	// gitExec reaches the git repo backing the jj store, which is needed for
	// fetching refs jj itself cannot express (pull request refs, most notably).
	gitExec GitExecutor
}

// NewWorkspaceManager creates a WorkspaceManager with the given executor.
func NewWorkspaceManager(executor Executor) *WorkspaceManager {
	return &WorkspaceManager{executor: executor, gitExec: &RealGitExecutor{}}
}

// Kind reports that this manager drives jj.
func (m *WorkspaceManager) Kind() vcs.Kind { return vcs.KindJJ }

// CurrentOperationID returns the most recent jj operation identity without
// mutating the working copy. It is intentionally a small inspection capability
// rather than part of vcs.Manager's mutation-oriented interface.
func (m *WorkspaceManager) CurrentOperationID(dir string) (string, error) {
	out, err := m.executor.Run(dir, "operation", "log", "--ignore-working-copy", "--no-graph", "-n", "1", "-T", `id ++ "\n"`)
	if err != nil {
		return "", fmt.Errorf("reading current jj operation: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// List returns every workspace in the repo, main first then alphabetical.
//
// --ignore-working-copy keeps listing side-effect free: without it jj snapshots
// the working copy and appends an operation to the log, which is the wrong
// behavior for a read-only command like `wtf ls`.
func (m *WorkspaceManager) List(dir string) ([]vcs.Worktree, error) {
	out, err := m.executor.Run(dir, "workspace", "list", "--ignore-working-copy", "-T", listTemplate)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}

	mainRoot, mainErr := MainRoot(dir)
	if mainErr != nil {
		// Fall back to no main marker rather than failing the whole listing.
		mainRoot = ""
	}

	wts := parseWorkspaceList(out, mainRoot)
	sortMainFirst(wts)
	return wts, nil
}

// parseWorkspaceList converts templated `jj workspace list` output into
// worktrees. mainRoot, when non-empty, marks which entry is the main workspace.
func parseWorkspaceList(output, mainRoot string) []vcs.Worktree {
	if strings.TrimSpace(output) == "" {
		return nil
	}

	var wts []vcs.Worktree
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, fieldSep)
		if len(fields) < 5 {
			continue
		}

		// JJ has one native name for a workspace. Keep Branch populated for
		// the legacy JSON/API contract, while exposing the same value as the
		// canonical WTF name and native backend name.
		name := fields[0]
		wt := vcs.Worktree{
			Name:       name,
			NativeName: name,
			Branch:     name,
			Path:       fields[1],
			Head:       fields[2],
			ChangeID:   fields[3],
			VCS:        vcs.KindJJ,
		}

		// A workspace whose directory was deleted still appears in the listing.
		// Depending on jj's version, self.root() renders either an error or an
		// empty path when it cannot resolve the directory. That is exactly wtf's
		// notion of prunable: the path is unrecoverable, but `jj workspace forget
		// <name>` still cleans up the registration.
		if wt.Path == "" || strings.HasPrefix(wt.Path, "<Error:") {
			wt.Path = ""
			wt.Prunable = true
		}

		if bm := strings.TrimSpace(fields[4]); bm != "" {
			wt.Bookmarks = strings.Split(bm, ",")
		}

		if mainRoot != "" && wt.Path != "" && sameDir(wt.Path, mainRoot) {
			wt.IsMain = true
		}

		wts = append(wts, wt)
	}
	return wts
}

// sortMainFirst hoists the main workspace to index 0. jj sorts workspaces
// alphabetically, so main is not naturally first the way it is in git — callers
// such as MainWorktree and `wtf ls` rely on the ordering.
func sortMainFirst(wts []vcs.Worktree) {
	sort.SliceStable(wts, func(i, j int) bool {
		if wts[i].IsMain != wts[j].IsMain {
			return wts[i].IsMain
		}
		return false
	})
}

// MainWorktree returns the main workspace — the one holding the jj repo.
func (m *WorkspaceManager) MainWorktree(dir string) (vcs.Worktree, error) {
	wts, err := m.List(dir)
	if err != nil {
		return vcs.Worktree{}, err
	}
	for _, wt := range wts {
		if wt.IsMain {
			return wt, nil
		}
	}
	if len(wts) == 0 {
		return vcs.Worktree{}, fmt.Errorf("no workspaces found")
	}
	return vcs.Worktree{}, fmt.Errorf("could not identify the main workspace of %s", dir)
}

// Find resolves a query to one workspace, preferring an exact name match and
// falling back to substring. Bookmarks are also matched exactly, so a workspace
// can be reached by a bookmark pointing at its working-copy commit.
func (m *WorkspaceManager) Find(dir, query string) (vcs.Worktree, error) {
	wts, err := m.List(dir)
	if err != nil {
		return vcs.Worktree{}, err
	}

	var matches []vcs.Worktree
	for _, wt := range wts {
		if wt.Branch == query {
			return wt, nil
		}
		for _, b := range wt.Bookmarks {
			if b == query {
				return wt, nil
			}
		}
		if strings.Contains(wt.Branch, query) {
			matches = append(matches, wt)
		}
	}

	switch len(matches) {
	case 0:
		return vcs.Worktree{}, fmt.Errorf("%w: %q", vcs.ErrWorktreeNotFound, query)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, mt := range matches {
			names = append(names, mt.Branch)
		}
		return vcs.Worktree{}, fmt.Errorf("%w %q: %s", vcs.ErrMultipleMatches, query, strings.Join(names, ", "))
	}
}

// Add creates a workspace named ref based on base, returning its path.
//
// No bookmark is created. jj enforces unique workspace names, so the name alone
// identifies the checkout; bookmarks stay a push-time concern the user controls
// via `jj bookmark create` or `jj git push -c`.
func (m *WorkspaceManager) Add(dir, ref, base string) (string, error) {
	if err := ValidateRef(ref); err != nil {
		return "", err
	}

	mainRoot, err := MainRoot(dir)
	if err != nil {
		return "", fmt.Errorf("finding main workspace: %w", err)
	}

	wsPath := vcs.WorktreePath(mainRoot, ref)

	args := []string{"workspace", "add", "--name", ref, wsPath}
	if rev := m.resolveBase(dir, base); rev != "" {
		args = append(args, "-r", rev)
	}

	if _, err := m.executor.Run(dir, args...); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "already exists") && strings.Contains(msg, "Workspace"):
			return "", fmt.Errorf("%w: workspace %q", vcs.ErrBranchAlreadyInUse, ref)
		case strings.Contains(msg, "Workspace named"):
			return "", fmt.Errorf("%w: workspace %q", vcs.ErrBranchAlreadyInUse, ref)
		case strings.Contains(msg, "already exists") || strings.Contains(msg, "not an empty directory"):
			return "", fmt.Errorf("%w: '%s'", vcs.ErrPathAlreadyExists, wsPath)
		case strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "Revision"):
			return "", fmt.Errorf("base revision %q does not resolve — pass --base with a bookmark or change id: %w", base, err)
		}
		return "", fmt.Errorf("adding workspace: %w", err)
	}

	return wsPath, nil
}

// resolveBase picks the revset for a new workspace. An empty base means the user
// did not ask for anything specific, so trunk() is preferred — jj's own notion of
// the main line — and otherwise the revset is omitted entirely so jj defaults to a
// sibling of the current working-copy commit.
func (m *WorkspaceManager) resolveBase(dir, base string) string {
	if base != "" {
		return base
	}
	if m.revsetUsable(dir, "trunk()") {
		return "trunk()"
	}
	return ""
}

// revsetUsable reports whether a revset names a commit worth basing a workspace
// on.
//
// The root-commit check is essential: in a repo with no remote, trunk() resolves
// to the root commit, whose tree is empty. Basing a workspace there produces a
// completely empty directory instead of a checkout of the project.
func (m *WorkspaceManager) revsetUsable(dir, revset string) bool {
	out, err := m.executor.Run(dir, "log", "--ignore-working-copy", "--no-graph",
		"-r", revset, "-T", `if(root, "root", "ok")`)
	if err != nil {
		return false
	}
	out = strings.TrimSpace(out)
	return out != "" && !strings.Contains(out, "root")
}

// revsetResolves reports whether a revset names at least one visible commit.
func (m *WorkspaceManager) revsetResolves(dir, revset string) bool {
	out, err := m.executor.Run(dir, "log", "--ignore-working-copy", "--no-graph",
		"-r", revset, "-T", `"x"`)
	return err == nil && strings.TrimSpace(out) != ""
}

// Remove forgets a workspace and deletes its directory.
//
// `jj workspace forget` only drops the registration — it never touches the
// directory, and the working-copy commit stays visible in `jj log` — so the
// directory removal is wtf's job.
func (m *WorkspaceManager) Remove(dir, ref, cwd string, force bool) error {
	wt, err := m.Find(dir, ref)
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	if wt.IsMain {
		return fmt.Errorf("%w: %q is the main workspace", vcs.ErrMainWorktree, wt.Branch)
	}

	if wt.Path != "" && vcs.IsInside(cwd, wt.Path) {
		return vcs.ErrWorktreeIsCurrentDir
	}

	if !force && wt.Path != "" {
		dirty, dErr := m.hasChanges(wt.Path)
		if dErr == nil && dirty {
			return fmt.Errorf("%w: use --force to remove anyway", vcs.ErrWorktreeHasChanges)
		}
	}

	if _, err := m.executor.Run(dir, "workspace", "forget", wt.Branch); err != nil {
		return fmt.Errorf("forgetting workspace %q: %w", wt.Branch, err)
	}

	if wt.Path != "" {
		if err := os.RemoveAll(wt.Path); err != nil {
			return fmt.Errorf("removing workspace directory %s: %w", wt.Path, err)
		}
	}

	return nil
}

// hasChanges reports whether the workspace at path has an uncommitted change in
// its working copy.
//
// The check must run with the workspace itself as the working directory: jj only
// snapshots the working copy it is invoked from, so asking from another
// workspace would report stale, usually-empty results.
func (m *WorkspaceManager) hasChanges(path string) (bool, error) {
	out, err := m.executor.Run(path, "log", "--no-graph", "-r", "@",
		"-T", `if(empty, "empty", "dirty")`)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "dirty", nil
}

// RemoteURL returns the URL of the "origin" remote.
func (m *WorkspaceManager) RemoteURL(dir string) (string, error) {
	out, err := m.executor.Run(dir, "git", "remote", "list")
	if err != nil {
		return "", fmt.Errorf("getting remote URL: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		name, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && name == "origin" {
			return strings.TrimSpace(url), nil
		}
	}
	return "", fmt.Errorf("no origin remote configured")
}

// StateDir returns the repo-scoped directory wtf keeps state in. It lives under
// the main workspace's .jj/repo so every workspace shares one location, matching
// how the git backend uses .git/wtf.
func (m *WorkspaceManager) StateDir(dir string) (string, error) {
	mainRoot, err := MainRoot(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(mainRoot, ".jj", "repo", "wtf"), nil
}

// CurrentRef returns the name of the workspace whose root is dir.
func (m *WorkspaceManager) CurrentRef(dir string) (string, error) {
	wts, err := m.List(dir)
	if err != nil {
		return "", err
	}
	for _, wt := range wts {
		if wt.Path != "" && sameDir(wt.Path, dir) {
			return wt.Branch, nil
		}
	}
	return "", fmt.Errorf("no workspace rooted at %s", dir)
}

// ValidateRef checks that ref is usable as a jj workspace name. jj accepts
// slashes, so feature/auth needs no rewriting; only genuinely unusable names are
// rejected here and jj is left to enforce the rest.
func ValidateRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("%w: workspace name cannot be empty", vcs.ErrInvalidRef)
	}
	if strings.ContainsAny(ref, " \t\n\r"+fieldSep) {
		return fmt.Errorf("%w: %q (whitespace is not allowed)", vcs.ErrInvalidRef, ref)
	}
	return nil
}

// MainRoot returns the root of the main workspace for the repo containing dir.
//
// In the main workspace .jj/repo is a directory. In every other workspace it is a
// file holding a path — relative to that workspace's .jj — pointing at the main
// workspace's .jj/repo, which is how a workspace with no .git of its own is
// traced back to the repo.
func MainRoot(dir string) (string, error) {
	root, err := WorkspaceRoot(dir)
	if err != nil {
		return "", err
	}

	repoMarker := filepath.Join(root, ".jj", "repo")
	info, err := os.Stat(repoMarker)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", repoMarker, err)
	}
	if info.IsDir() {
		return root, nil
	}

	data, err := os.ReadFile(repoMarker)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", repoMarker, err)
	}

	target := strings.TrimSpace(string(data))
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, ".jj", target)
	}
	resolved, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", fmt.Errorf("resolving jj repo path %q: %w", target, err)
	}

	// resolved points at <mainRoot>/.jj/repo.
	return filepath.Dir(filepath.Dir(resolved)), nil
}

// WorkspaceRoot walks up from dir to the nearest directory containing .jj.
func WorkspaceRoot(dir string) (string, error) {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(cur, ".jj")); err == nil && info.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", vcs.ErrNotARepo
		}
		cur = parent
	}
}

// sameDir compares two paths, tolerating symlinked parents (/tmp → /private/tmp
// on macOS, for instance).
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return ra == rb
}

// Assert at compile time that WorkspaceManager satisfies the shared interface.
var _ vcs.Manager = (*WorkspaceManager)(nil)

// FetchRefspec fetches a "src:dst" refspec and makes dst resolvable as a revset.
//
// It goes through the git repo backing the jj store rather than `jj git fetch`,
// because jj can only fetch bookmarks — it has no way to express a refspec like
// `pull/42/head:pr-42`, which is exactly what forge PR checkout needs. After the
// fetch, `jj git import` pulls the new ref into jj as a bookmark so `dst` works as
// a base revision.
func (m *WorkspaceManager) FetchRefspec(dir, remote, refspec string) error {
	gitDir, err := GitDir(dir)
	if err != nil {
		return err
	}

	if _, err := m.gitExec.Run(gitDir, "fetch", remote, refspec); err != nil {
		return fmt.Errorf("fetching %s from %s: %w", refspec, remote, err)
	}

	if _, err := m.executor.Run(dir, "git", "import"); err != nil {
		return fmt.Errorf("importing fetched refs into jj: %w", err)
	}

	return nil
}

// GitDir returns the git repository backing the jj store for the repo at dir.
//
// The location is recorded in .jj/repo/store/git_target, relative to that store
// directory. A colocated repo points at the top-level .git; a non-colocated one
// points inside .jj, which is why plain git cannot find it.
func GitDir(dir string) (string, error) {
	mainRoot, err := MainRoot(dir)
	if err != nil {
		return "", err
	}

	storeDir := filepath.Join(mainRoot, ".jj", "repo", "store")
	data, err := os.ReadFile(filepath.Join(storeDir, "git_target"))
	if err != nil {
		return "", fmt.Errorf("reading jj git target: %w", err)
	}

	target := strings.TrimSpace(string(data))
	if target == "" {
		return "", fmt.Errorf("jj git target is empty in %s", storeDir)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(storeDir, target)
	}
	return filepath.Clean(target), nil
}

// Cleanable returns workspaces that are safe to discard: those whose directory is
// already gone, and those holding no work that is not already on the main line.
//
// jj has no "merged branch" to test, so the analogue is a working-copy commit that
// is empty *and* whose parent is an ancestor of trunk() — nothing in progress and
// nothing left to land. Anything with real content is left alone.
func (m *WorkspaceManager) Cleanable(dir string) ([]vcs.Worktree, error) {
	wts, err := m.List(dir)
	if err != nil {
		return nil, err
	}

	hasTrunk := m.revsetResolves(dir, "trunk()")

	var out []vcs.Worktree
	for _, wt := range wts {
		if wt.IsMain {
			continue
		}
		if wt.Prunable {
			out = append(out, wt)
			continue
		}
		if hasTrunk && m.isSpent(dir, wt.Branch) {
			out = append(out, wt)
		}
	}
	return out, nil
}

// isSpent reports whether a workspace's working-copy commit is empty and sits on
// top of something already contained in trunk().
func (m *WorkspaceManager) isSpent(dir, name string) bool {
	revset := fmt.Sprintf("%s@", name)
	out, err := m.executor.Run(dir, "log", "--ignore-working-copy", "--no-graph",
		"-r", revset,
		"-T", `if(empty, "empty", "dirty") ++ ":" ++ if(parents.all(|p| p.contained_in("::trunk()")), "landed", "ahead")`)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "empty:landed"
}
