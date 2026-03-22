package git

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for branch operations.
var (
	ErrBranchAlreadyExists = errors.New("branch already exists")
	ErrInvalidBranchName   = errors.New("invalid branch name")
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

// ValidateBranchName checks if a branch name is valid using git check-ref-format.
func (bm *BranchManager) ValidateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: branch name cannot be empty", ErrInvalidBranchName)
	}
	_, err := bm.executor.Run(".", "check-ref-format", "--allow-onelevel", name)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidBranchName, name)
	}
	return nil
}

// MergedBranches returns branches whose unique commits have been merged into
// the given base branch. Branches that sit at the exact same commit as base
// are excluded — they never diverged, so they are not "merged".
func (bm *BranchManager) MergedBranches(dir, base string) ([]string, error) {
	out, err := bm.executor.Run(dir, "branch", "--merged", base)
	if err != nil {
		return nil, fmt.Errorf("listing merged branches: %w", err)
	}

	baseRev, err := bm.executor.Run(dir, "rev-parse", base)
	if err != nil {
		return nil, fmt.Errorf("resolving base branch %s: %w", base, err)
	}
	baseRev = strings.TrimSpace(baseRev)

	var branches []string
	for _, line := range strings.Split(out, "\n") {
		// Strip leading markers: * (current), + (checked out in another worktree)
		name := strings.TrimSpace(line)
		name = strings.TrimLeft(name, "*+ ")
		name = strings.TrimSpace(name)
		if name == "" || name == base {
			continue
		}

		// Exclude branches at the same commit as base — they never diverged.
		branchRev, err := bm.executor.Run(dir, "rev-parse", name)
		if err != nil {
			continue
		}
		if strings.TrimSpace(branchRev) == baseRev {
			continue
		}

		branches = append(branches, name)
	}
	return branches, nil
}
