package git

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// Sentinel errors for worktree operations. These alias the shared vcs errors so
// that errors.Is works against either package's name — the jj backend wraps the
// same values, letting internal/cli render one set of messages for both.
var (
	ErrWorktreeNotFound     = vcs.ErrWorktreeNotFound
	ErrMultipleMatches      = vcs.ErrMultipleMatches
	ErrMainWorktree         = vcs.ErrMainWorktree
	ErrWorktreeIsCurrentDir = vcs.ErrWorktreeIsCurrentDir
	ErrBranchAlreadyInUse   = vcs.ErrBranchAlreadyInUse
	ErrPathAlreadyExists    = vcs.ErrPathAlreadyExists
	ErrWorktreeHasChanges   = vcs.ErrWorktreeHasChanges
)

// Worktree represents a single git worktree entry. It is an alias of the shared
// vcs model so git and jj listings are the same type end to end.
type Worktree = vcs.Worktree

// WorktreeManager handles worktree operations.
type WorktreeManager struct {
	executor Executor
}

// NewWorktreeManager creates a new WorktreeManager with the given executor.
func NewWorktreeManager(executor Executor) *WorktreeManager {
	return &WorktreeManager{executor: executor}
}

// List returns all worktrees for the repo at dir.
func (wm *WorktreeManager) List(dir string) ([]Worktree, error) {
	out, err := wm.executor.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	return parseWorktreeList(out)
}

// Kind reports that this manager drives git. Part of the vcs.Manager interface.
func (wm *WorktreeManager) Kind() vcs.Kind { return vcs.KindGit }

// StateDir returns the shared .git/wtf directory used for wtf's repo-local
// state. It resolves via --git-common-dir so every worktree of a repo agrees on
// one location.
func (wm *WorktreeManager) StateDir(dir string) (string, error) {
	commonDir, err := wm.executor.Run(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("getting git common dir: %w", err)
	}
	// git may return a path relative to the worktree.
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	return filepath.Join(commonDir, "wtf"), nil
}

// CurrentRef returns the branch checked out at dir.
func (wm *WorktreeManager) CurrentRef(dir string) (string, error) {
	out, err := wm.executor.Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detecting current branch: %w", err)
	}
	return out, nil
}

// MainWorktree returns the main (first) worktree.
func (wm *WorktreeManager) MainWorktree(dir string) (Worktree, error) {
	wts, err := wm.List(dir)
	if err != nil {
		return Worktree{}, err
	}
	if len(wts) == 0 {
		return Worktree{}, fmt.Errorf("no worktrees found")
	}
	return wts[0], nil
}

// Add creates a new worktree for the given branch.
func (wm *WorktreeManager) Add(dir, branch, base string) (string, error) {
	mainWt, err := wm.MainWorktree(dir)
	if err != nil {
		return "", fmt.Errorf("finding main worktree: %w", err)
	}

	wtPath := WorktreePath(mainWt.Path, branch)

	args := []string{"worktree", "add", wtPath, branch}

	// Check if branch exists
	bm := NewBranchManager(wm.executor)
	exists, err := bm.Exists(dir, branch)
	if err != nil {
		return "", fmt.Errorf("checking branch: %w", err)
	}

	if !exists {
		// Create branch from base, then add worktree
		args = []string{"worktree", "add", "-b", branch, wtPath, base}
	}

	if _, err := wm.executor.Run(dir, args...); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "is already used by worktree") {
			// Extract the worktree path from git's error message
			if idx := strings.Index(errMsg, "already used by worktree at '"); idx != -1 {
				rest := errMsg[idx+len("already used by worktree at '"):]
				if end := strings.Index(rest, "'"); end != -1 {
					return "", fmt.Errorf("%w: '%s' is already used by worktree at %s", ErrBranchAlreadyInUse, branch, rest[:end])
				}
			}
			return "", fmt.Errorf("%w: '%s'", ErrBranchAlreadyInUse, branch)
		}
		if strings.Contains(errMsg, "already exists") {
			return "", fmt.Errorf("%w: '%s'", ErrPathAlreadyExists, wtPath)
		}
		return "", fmt.Errorf("adding worktree: %w", err)
	}

	return wtPath, nil
}

// Find searches worktrees by substring match on branch name.
// Returns the matching worktree, or an error if zero or multiple matches.
func (wm *WorktreeManager) Find(dir, query string) (Worktree, error) {
	wts, err := wm.List(dir)
	if err != nil {
		return Worktree{}, err
	}

	var matches []Worktree
	for _, wt := range wts {
		if wt.Branch == query {
			return wt, nil
		}
		if strings.Contains(wt.Branch, query) {
			matches = append(matches, wt)
		}
	}

	switch len(matches) {
	case 0:
		return Worktree{}, fmt.Errorf("%w: %q", ErrWorktreeNotFound, query)
	case 1:
		return matches[0], nil
	default:
		var branches []string
		for _, m := range matches {
			branches = append(branches, m.Branch)
		}
		return Worktree{}, fmt.Errorf("%w %q: %s", ErrMultipleMatches, query, strings.Join(branches, ", "))
	}
}

// Remove removes a worktree without deleting its branch.
// cwd is the caller's current working directory; removal is blocked if
// cwd falls inside the target worktree.
func (wm *WorktreeManager) Remove(dir, branch, cwd string, force bool) error {
	wt, err := wm.Find(dir, branch)
	if err != nil {
		return fmt.Errorf("finding worktree: %w", err)
	}

	if wt.IsMain {
		return ErrMainWorktree
	}

	if isInsideWorktree(cwd, wt.Path) {
		return ErrWorktreeIsCurrentDir
	}

	args := []string{"worktree", "remove", wt.Path}
	if force {
		args = append(args, "--force")
	}

	if _, err := wm.executor.Run(dir, args...); err != nil {
		if strings.Contains(err.Error(), "modified or untracked files") {
			return fmt.Errorf("%w: use --force to remove anyway", ErrWorktreeHasChanges)
		}
		return fmt.Errorf("removing worktree: %w", err)
	}

	return nil
}

// FetchRefspec fetches a "src:dst" refspec from a remote.
func (wm *WorktreeManager) FetchRefspec(dir, remote, refspec string) error {
	if _, err := wm.executor.Run(dir, "fetch", remote, refspec); err != nil {
		return fmt.Errorf("fetching %s from %s: %w", refspec, remote, err)
	}
	return nil
}

// Cleanable returns worktrees whose branch has been merged into the main
// worktree's branch, plus worktrees git reports as prunable.
func (wm *WorktreeManager) Cleanable(dir string) ([]Worktree, error) {
	wts, err := wm.List(dir)
	if err != nil {
		return nil, err
	}

	mainBranch := "main"
	for _, wt := range wts {
		if wt.IsMain {
			mainBranch = wt.Branch
			break
		}
	}

	bm := NewBranchManager(wm.executor)
	merged, err := bm.MergedBranches(dir, mainBranch)
	if err != nil {
		return nil, err
	}

	mergedSet := make(map[string]bool, len(merged))
	for _, b := range merged {
		mergedSet[b] = true
	}

	var out []Worktree
	for _, wt := range wts {
		if wt.IsMain {
			continue
		}
		if wt.Prunable || mergedSet[wt.Branch] {
			out = append(out, wt)
		}
	}
	return out, nil
}

// parseWorktreeList parses `git worktree list --porcelain` output.
func parseWorktreeList(output string) ([]Worktree, error) {
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}

	var worktrees []Worktree
	var current Worktree
	isFirst := true

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				current.IsMain = isFirst
				worktrees = append(worktrees, current)
				isFirst = false
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree "), VCS: vcs.KindGit}
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			current.IsBare = true
		case line == "detached":
			current.IsDetached = true
		case strings.HasPrefix(line, "prunable "):
			current.Prunable = true
		case line == "":
			// block separator — handled by next "worktree" line
		}
	}

	// Append last entry
	if current.Path != "" {
		current.IsMain = isFirst
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// isInsideWorktree reports whether cwd is equal to or a subdirectory of wtPath.
func isInsideWorktree(cwd, wtPath string) bool {
	return vcs.IsInside(cwd, wtPath)
}

// WorktreePath computes the sibling worktree directory path. Aliased from vcs so
// git and jj lay out their checkouts identically.
var WorktreePath = vcs.WorktreePath
