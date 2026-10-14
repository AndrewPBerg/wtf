package cli

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/x/ansi"
)

// Nested workspace labels show their placement relative to the repository anchor.
// Identity and selectors remain the native JJ workspace name.
func workspaceDisplayName(row core.WorkspaceView) string {
	if !filepath.IsAbs(row.RepositoryPath) || !filepath.IsAbs(row.Path) {
		return row.Name
	}
	relative, err := filepath.Rel(row.RepositoryPath, row.Path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return row.Name
	}
	return "./" + filepath.ToSlash(relative)
}

// The visible name can be truncated; the editor always receives the
// exact absolute workspace root. Missing and removed records are not actionable.
func workspaceLinkPresentation(p presentation, row core.WorkspaceView, label string) string {
	return workspaceStyledLink(p, row, plain(label))
}

// Labels here have already been sanitized before adding theme styles.
func workspaceStyledLink(p presentation, row core.WorkspaceView, label string) string {
	if !p.Links || !row.Exists || row.Status == "removed" || !filepath.IsAbs(row.Path) || plain(row.Path) != row.Path {
		return label
	}
	target := (&url.URL{Scheme: "zed", Host: "file", Path: filepath.ToSlash(row.Path)}).String()
	return ansi.SetHyperlink(target) + label + ansi.ResetHyperlink()
}

func workspaceSearchLink(p presentation, row core.WorkspaceView, search pickerSearch) string {
	return workspaceStyledLink(p, row, search.highlight(p, workspaceDisplayName(row)))
}
