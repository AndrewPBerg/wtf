package core

import "context"

// removeFromParent resolves within the same scope as switch. Resolve the entire
// scope before mutating anything: names may be repeated across repositories.
func (a *App) removeFromParent(ctx context.Context, name string, options RemovalOptions) (Workspace, error) {
	rows, err := a.List(ctx, false, false)
	if err != nil {
		return Workspace{}, err
	}
	var target WorkspaceView
	found := false
	for _, row := range rows {
		if row.Status == "removed" || row.Name != name && row.ID != name {
			continue
		}
		if found {
			return Workspace{}, Fail("ambiguous_workspace", "Multiple repositories have workspace "+name+"; use its workspace ID or run inside its repository")
		}
		target, found = row, true
	}
	if !found {
		return Workspace{}, Fail("workspace_missing", "No managed workspace matches "+name)
	}
	app := *a
	app.Cwd = target.RepositoryPath
	var removed Workspace
	_, err = app.InvokeHooks(ctx, HookInvocation{Command: "remove", Args: []string{target.ID}}, func() (any, error) {
		var err error
		removed, err = app.RemoveWithOptions(ctx, target.ID, options)
		return removed, err
	})
	return removed, err
}
