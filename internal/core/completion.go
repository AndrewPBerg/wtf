package core

import (
	"context"
	"path/filepath"
	"strings"
)

// CompletionWorkspaces reads local registries without reconciliation or mutation.
func (a *App) CompletionWorkspaces(ctx context.Context, global, here bool) ([]WorkspaceView, error) {
	if !global {
		if r, err := Discover(ctx, a.Cwd); err == nil {
			state, err := r.Load()
			if err != nil {
				return nil, err
			}
			rows := views(state)
			if here {
				selected := rows[:0]
				for _, row := range rows {
					if samePath(row.Path, r.Root) {
						selected = append(selected, row)
					}
				}
				rows = selected
			}
			return rows, nil
		}
	}
	index, err := a.index()
	if err != nil {
		return nil, err
	}
	rows := []WorkspaceView{}
	for _, entry := range index.Repositories {
		if !global {
			rel, err := filepath.Rel(a.Cwd, entry.Path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		state, err := (Repo{entry.Path, entry.Store}).Load()
		if err != nil {
			continue
		} // A stale repository must not hide healthy candidates.
		rows = append(rows, views(state)...)
	}
	return rows, nil
}
