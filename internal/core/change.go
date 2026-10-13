package core

import (
	"context"
	"os"
	"strings"
)

// ChangeWorkspace replaces the message on a registered workspace. An empty
// name selects the current workspace; an empty message clears its description.
func (a *App) ChangeWorkspace(ctx context.Context, name, message string) (Workspace, error) {
	if strings.ContainsRune(message, 0) {
		return Workspace{}, Fail("invalid_message", "Workspace messages cannot contain NUL characters")
	}
	r, err := Discover(ctx, a.Cwd)
	if err != nil {
		return Workspace{}, err
	}
	var w Workspace
	if name == "" {
		w, err = r.Workspace(ctx)
	} else {
		w, err = r.Select(name)
	}
	if err != nil {
		return Workspace{}, err
	}
	unlock, err := r.LockWorkspace(ctx, w.ID)
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	err = r.Edit(ctx, func(state *State) error {
		current, ok := state.Workspaces[w.ID]
		if !ok || current.Status == "removed" {
			return Fail("workspace_missing", "Workspace identity is missing or removed")
		}
		info, err := os.Stat(current.Path)
		if err != nil || !info.IsDir() {
			return Fail("workspace_unavailable", "Workspace path is unavailable")
		}
		current.Message = message
		state.Workspaces[w.ID] = current
		w = current
		return nil
	})
	return w, err
}
