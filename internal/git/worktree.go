package git

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Sentinel errors for worktree operations.
var (
	ErrWorktreeNotFound     = errors.New("no matching worktree found")
	ErrMultipleMatches      = errors.New("multiple worktrees match query")
	ErrMainWorktree         = errors.New("cannot remove main worktree")
	ErrWorktreeIsCurrentDir = errors.New("cannot remove worktree for the currently checked out branch")
	ErrBranchAlreadyInUse   = errors.New("branch is already checked out")
	ErrPathAlreadyExists    = errors.New("worktree path already exists")
	ErrWorktreeHasChanges   = errors.New("worktree has uncommitted changes")
)

// Worktree represents a single git worktree entry.
type Worktree struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	Head       string `json:"head"`
	IsMain     bool   `json:"is_main"`
	IsBare     bool   `json:"is_bare"`
	IsDetached bool   `json:"is_detached"`
	Prunable   bool   `json:"prunable"`
}

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

// Remove removes a worktree and optionally deletes the branch.
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

	// Delete the branch
	deleteFlag := "-d"
	if force {
		deleteFlag = "-D"
	}
	if _, err := wm.executor.Run(dir, "branch", deleteFlag, wt.Branch); err != nil {
		return fmt.Errorf("deleting branch %s: %w", wt.Branch, err)
	}

	return nil
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
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
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
	// Clean both paths so trailing slashes, ".." etc. are normalised.
	cwd = filepath.Clean(cwd)
	wtPath = filepath.Clean(wtPath)

	if cwd == wtPath {
		return true
	}
	return strings.HasPrefix(cwd, wtPath+string(filepath.Separator))
}

// WorktreePath computes the sibling worktree directory path.
// /code/myrepo + feature/auth → /code/myrepo--feature-auth
func WorktreePath(mainPath, branch string) string {
	sanitized := strings.ReplaceAll(branch, "/", "-")
	parent := filepath.Dir(mainPath)
	base := filepath.Base(mainPath)
	return filepath.Join(parent, base+"--"+sanitized)
}
