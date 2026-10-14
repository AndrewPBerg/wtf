package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestWorkspaceNamesLinkToExactZedRootAcrossLayouts(t *testing.T) {
	row := core.WorkspaceView{Workspace: core.Workspace{Name: "feature", Path: "/repo/Feature space/#日本%?", Status: "ready"}, Exists: true}
	want := "\x1b]8;;zed://file/repo/Feature%20space/%23%E6%97%A5%E6%9C%AC%25%3F\x07"
	p := humanPresentation(&bytes.Buffer{})
	p.Links = true
	for _, width := range []int{34, 80, 120} {
		p.Width = width
		m := newBrowser([]core.WorkspaceView{row}, "/repo", p, false, false)
		removal := newRemovalPicker([]core.RemovalCandidate{{WorkspaceView: row}}, "/repo", p)
		for _, view := range []string{m.View(), workspaceTable(p, []core.WorkspaceView{row}, "/repo", false, false), removal.View()} {
			if !strings.Contains(view, want) || !strings.Contains(view, ansi.ResetHyperlink()) {
				t.Fatalf("width %d lost the absolute editor link: %q", width, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > width-1 {
					t.Fatalf("link expanded width %d: %q", width, line)
				}
			}
			if strings.Contains(ansi.Strip(view), "PATH") || strings.Contains(ansi.Strip(view), row.Path) || !strings.Contains(ansi.Strip(view), row.Name) {
				t.Fatalf("workspace name did not replace the path: %q", view)
			}
			p.Links = false
			plainBrowser := newBrowser([]core.WorkspaceView{row}, "/repo", p, false, false)
			plainView := workspaceTable(p, []core.WorkspaceView{row}, "/repo", false, false)
			plainRemoval := newRemovalPicker([]core.RemovalCandidate{{WorkspaceView: row}}, "/repo", p)
			if strings.Contains(view, "WORKSPACE") && ansi.Strip(view) != ansi.Strip(plainView) && ansi.Strip(view) != ansi.Strip(plainBrowser.View()) && ansi.Strip(view) != ansi.Strip(plainRemoval.View()) {
				t.Fatalf("links changed visible layout: %q", view)
			}
			p.Links = true
		}
	}
}

func TestWorkspaceNamesSuppressLinksForUnactionableRecords(t *testing.T) {
	p := humanPresentation(&bytes.Buffer{})
	p.Links = true
	for _, row := range []core.WorkspaceView{
		{Workspace: core.Workspace{Path: "/repo/missing"}},
		{Workspace: core.Workspace{Path: "/repo/removed", Status: "removed"}, Exists: true},
		{Workspace: core.Workspace{Path: "relative"}, Exists: true},
		{Workspace: core.Workspace{Path: "/repo/\x1b]8;;injected"}, Exists: true},
	} {
		if got := workspaceLinkPresentation(p, row, "label"); got != "label" {
			t.Fatalf("unactionable path linked: %q", got)
		}
	}
	p.Links = false
	row := core.WorkspaceView{Workspace: core.Workspace{Path: "/repo/feature"}, Exists: true}
	if got := workspaceLinkPresentation(p, row, "feature"); got != "feature" {
		t.Fatalf("redirected output linked: %q", got)
	}
}

func TestNestedWorkspacePlacementIsVisibleAcrossLayouts(t *testing.T) {
	p := humanPresentation(&bytes.Buffer{})
	p.Width = 100
	for _, test := range []struct{ name, path, want string }{
		{"default", "/projects/repo", "default"},
		{"flat", "/projects/flat", "flat"},
		{"hello_vert", "/projects/repo/hello_vert", "./hello_vert"},
		{"deep", "/projects/repo/work/deep", "./work/deep"},
		{"outside", "/projects/repo-other/outside", "outside"},
	} {
		row := core.WorkspaceView{Workspace: core.Workspace{ID: test.name, Name: test.name, Path: test.path, Status: "ready"}, RepositoryPath: "/projects/repo", Exists: true}
		if got := workspaceDisplayName(row); got != test.want {
			t.Fatalf("%s: label %q; want %q", test.path, got, test.want)
		}
		picker := newBrowser([]core.WorkspaceView{row}, "/projects/repo", p, false, false)
		removal := newRemovalPicker([]core.RemovalCandidate{{WorkspaceView: row}}, "/projects/repo", p)
		for _, view := range []string{picker.View(), workspaceTable(p, []core.WorkspaceView{row}, "/projects/repo", false, false), removal.View()} {
			if !strings.Contains(ansi.Strip(view), test.want) {
				t.Fatalf("placement label missing: %q", view)
			}
		}
		next, _ := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
		selected := next.(browser)
		if selected.chosen == nil || selected.chosen.Name != test.name || selected.chosen.Path != test.path {
			t.Fatalf("display changed selection identity: %+v", selected.chosen)
		}
	}
}
