package vcs

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/identity"
)

// EnrichWorktrees joins a backend listing to the repository selected by its
// backend-owned marker. It is read-only and deliberately leaves legacy entries
// untouched when markerID is empty.
func EnrichWorktrees(state identity.State, markerID string, backend Kind, worktrees []Worktree) ([]Worktree, error) {
	if markerID == "" {
		return append([]Worktree(nil), worktrees...), nil
	}
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("validating identity state: %w", err)
	}
	if err := identity.ValidateID(markerID); err != nil {
		return nil, fmt.Errorf("validating repository marker: %w", err)
	}

	foundRepo := false
	for _, repo := range state.Repositories {
		if repo.ID == markerID {
			if repo.LifecycleState != identity.Active {
				return nil, fmt.Errorf("repository marker %q does not identify an active repository", markerID)
			}
			foundRepo = true
			break
		}
	}
	if !foundRepo {
		return nil, fmt.Errorf("repository marker %q conflicts with identity state", markerID)
	}

	byPath := make(map[string]identity.Workspace)
	for _, workspace := range state.Workspaces {
		if workspace.RepositoryID != markerID || workspace.LifecycleState != identity.Active {
			continue
		}
		path, err := identity.CanonicalPhysicalPath(workspace.Path)
		if err != nil {
			return nil, fmt.Errorf("canonicalizing identity workspace %q: %w", workspace.ID, err)
		}
		if Kind(workspace.Backend) == backend {
			byPath[path] = workspace
		}
	}

	out := append([]Worktree(nil), worktrees...)
	for i := range out {
		if out[i].Path == "" || out[i].Prunable {
			continue
		}
		if _, err := os.Stat(out[i].Path); err != nil {
			continue
		}
		path, err := identity.CanonicalPhysicalPath(out[i].Path)
		if err != nil {
			continue
		}
		workspace, ok := byPath[path]
		if !ok {
			// In a colocated repository the other backend may legitimately own an
			// identity at this physical path. That is projection overlap, not a
			// conflict; leave this row legacy-shaped and never cross-enrich it.
			continue
		}
		out[i].RepositoryID = workspace.RepositoryID
		out[i].WorkspaceID = workspace.ID
		out[i].Name = workspace.Name
		out[i].NativeName = workspace.NativeName
	}
	return out, nil
}

// FindWorkspaceByID selects exactly one worktree by its stable workspace ID.
func FindWorkspaceByID(worktrees []Worktree, id string) (Worktree, error) {
	for _, worktree := range worktrees {
		if worktree.WorkspaceID == id {
			return worktree, nil
		}
	}
	return Worktree{}, fmt.Errorf("%w: workspace ID %q", ErrWorktreeNotFound, id)
}
