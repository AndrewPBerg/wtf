package git

import (
	"fmt"
	"strings"
)

// BranchManager handles branch-related operations.
type BranchManager struct {
	executor Executor
}

// NewBranchManager creates a new BranchManager.
func NewBranchManager(executor Executor) *BranchManager {
	return &BranchManager{executor: executor}
}

// Exists checks if a branch exists in the repo.
func (bm *BranchManager) Exists(dir, branch string) (bool, error) {
	out, err := bm.executor.Run(dir, "branch", "--list", branch)
	if err != nil {
		return false, fmt.Errorf("checking branch %s: %w", branch, err)
	}
	return strings.TrimSpace(out) != "", nil
}

// CurrentBranch returns the current branch name.
func (bm *BranchManager) CurrentBranch(dir string) (string, error) {
	out, err := bm.executor.Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	return out, nil
}

// MergedBranches returns branches merged into the given base branch.
func (bm *BranchManager) MergedBranches(dir, base string) ([]string, error) {
	out, err := bm.executor.Run(dir, "branch", "--merged", base)
	if err != nil {
		return nil, fmt.Errorf("listing merged branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(out, "\n") {
		// Strip leading markers: * (current), + (checked out in another worktree)
		name := strings.TrimSpace(line)
		name = strings.TrimLeft(name, "*+ ")
		name = strings.TrimSpace(name)
		if name != "" && name != base {
			branches = append(branches, name)
		}
	}
	return branches, nil
}
