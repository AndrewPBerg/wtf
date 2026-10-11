package cli

import (
	"net/url"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/x/ansi"
)

// The visible name can be truncated; the editor always receives the
// exact absolute workspace root. Missing and removed records are not actionable.
func workspaceLinkPresentation(p presentation, row core.WorkspaceView, label string) string {
	label = plain(label)
	if !p.Links || !row.Exists || row.Status == "removed" || !filepath.IsAbs(row.Path) || plain(row.Path) != row.Path {
		return label
	}
	target := (&url.URL{Scheme: "zed", Host: "file", Path: filepath.ToSlash(row.Path)}).String()
	return ansi.SetHyperlink(target) + label + ansi.ResetHyperlink()
}
