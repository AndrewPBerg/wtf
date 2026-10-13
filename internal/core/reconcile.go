package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func missingWorkspace(w Workspace, state State) bool {
	if w.Status == "removed" || w.Status == "creating" {
		return false
	}
	for _, claim := range state.Claims {
		if claim.Owner == w.ID {
			return false // Retain resource ownership until explicit cleanup.
		}
	}
	_, err := os.Lstat(w.Path)
	return errors.Is(err, os.ErrNotExist)
}

// loadReconciled forgets absent, idle workspace registrations while loading a
// list. Permission errors, broken symlinks, and resource owners are not absence.
func (r Repo) loadReconciled(ctx context.Context) (State, error) {
	state, err := r.Load()
	if err != nil {
		return state, err
	}
	missing := []string{}
	for id, workspace := range state.Workspaces {
		if missingWorkspace(workspace, state) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return state, nil
	}
	// Global listing can start from an unavailable anchor; use a surviving root.
	root := r.Root
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		for _, workspace := range state.Workspaces {
			if info, statErr := os.Stat(workspace.Path); statErr == nil && info.IsDir() {
				root = workspace.Path
				break
			}
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		unlock, lockErr := r.LockWorkspace(bounded, id)
		cancel()
		if lockErr != nil {
			if ctx.Err() != nil {
				return state, ctx.Err()
			}
			if errors.Is(lockErr, context.DeadlineExceeded) {
				continue // An in-flight creation/removal owns its registry update.
			}
			return state, lockErr
		}
		err = r.Edit(ctx, func(current *State) error {
			workspace, ok := current.Workspaces[id]
			if !ok || !missingWorkspace(workspace, *current) {
				return nil
			}
			lookup, stop := context.WithTimeout(ctx, 3*time.Second)
			defer stop()
			native, nativeErr := NativeWorkspaces(lookup, root)
			if nativeErr != nil {
				return nativeErr
			}
			metadata, unlocked, gitErr := missingGitWorktreeMetadata(lookup, root, workspace.Path)
			if gitErr != nil {
				return gitErr
			}
			if !unlocked || !missingWorkspace(workspace, *current) {
				return nil
			}
			if path, exists := native[workspace.Name]; exists && filepath.Clean(path) == filepath.Clean(workspace.Path) {
				if _, forgetErr := output(lookup, root, "workspace", "forget", "--ignore-working-copy", "--", workspace.Name); forgetErr != nil {
					return forgetErr
				}
			}
			if metadata != "" {
				// Only this absent workspace's administrative entry is removed.
				// Neither its source directory nor other Git worktrees are touched.
				if removeErr := os.RemoveAll(metadata); removeErr != nil {
					return removeErr
				}
			}
			delete(current.Workspaces, id)
			return nil
		})
		unlock()
		if err != nil {
			return state, err
		}
	}
	return r.Load()
}

func missingGitWorktreeMetadata(ctx context.Context, root, path string) (string, bool, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
		return "", true, nil // Non-colocated JJ repositories have no Git worktrees.
	} else if err != nil {
		return "", false, err
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return "", false, Fail("git_failed", "Cannot inspect native Git worktree registrations")
	}
	common := strings.TrimSpace(string(data))
	if !filepath.IsAbs(common) {
		return "", false, Fail("git_failed", "Git returned an invalid administrative directory")
	}
	worktrees := filepath.Join(common, "worktrees")
	entries, err := os.ReadDir(worktrees)
	if errors.Is(err, os.ErrNotExist) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		admin := filepath.Join(worktrees, entry.Name())
		reference, readErr := os.ReadFile(filepath.Join(admin, "gitdir"))
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return "", false, readErr
		}
		referencePath := strings.TrimSpace(string(reference))
		if !filepath.IsAbs(referencePath) {
			referencePath = filepath.Join(admin, referencePath)
		}
		if filepath.Clean(referencePath) != filepath.Join(path, ".git") {
			continue
		}
		if _, lockErr := os.Lstat(filepath.Join(admin, "locked")); lockErr == nil {
			return "", false, nil
		} else if !errors.Is(lockErr, os.ErrNotExist) {
			return "", false, lockErr
		}
		return admin, true, nil
	}
	return "", true, nil
}
