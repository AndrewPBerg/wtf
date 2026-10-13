package core

import (
	"context"
	"path/filepath"
	"strings"
)

// ResolveWorkspacePath shares destination semantics between native creation and
// hook events: bare names are siblings of the anchor, explicit paths use cwd.
func (a *App) ResolveWorkspacePath(ctx context.Context, path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", Fail("invalid_path", "A workspace destination path is required")
	}
	if !filepath.IsAbs(path) {
		base := a.Cwd
		if path != "." && path != ".." && !strings.ContainsRune(path, filepath.Separator) {
			r, err := Discover(ctx, a.Cwd)
			if err == nil {
				s, err := r.Load()
				if err != nil {
					return "", err
				}
				base = filepath.Dir(s.Anchor)
			} else if ErrorCode(err) != "not_a_jj_repository" {
				return "", err
			}
		}
		path = filepath.Join(base, path)
	}
	return filepath.Abs(path)
}
