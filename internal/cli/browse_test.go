package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestBrowserPagesResizesAndSelectsWithoutEnteringRemovedPaths(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	rows := []core.WorkspaceView{}
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven"} {
		rows = append(rows, core.WorkspaceView{Workspace: core.Workspace{Name: name, Path: "/repo/" + name, Status: "ready"}, Exists: true})
	}
	rows = append(rows, core.WorkspaceView{Workspace: core.Workspace{Name: "removed", Path: "/gone", Status: "removed"}})
	m := newBrowser(rows, "/repo/one", humanPresentation(&bytes.Buffer{}), true, true)
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(msg)
		var ok bool
		m, ok = next.(browser)
		if !ok {
			t.Fatal("invalid browser model")
		}
		return cmd
	}
	update(tea.WindowSizeMsg{Width: 60, Height: 10})
	if !strings.Contains(m.View(), "1/3") || strings.Contains(m.View(), "four") {
		t.Fatalf("first page incorrect: %s", m.View())
	}
	update(tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(m.View(), "2/3") || !strings.Contains(m.View(), "four") {
		t.Fatalf("second page incorrect: %s", m.View())
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 34, Height: 10}, {Width: 9, Height: 5}, {Width: 3, Height: 2}} {
		update(size)
		view := m.View()
		if lipgloss.Height(view) > size.Height {
			t.Fatalf("view exceeded %d rows: %s", size.Height, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.Width-1 {
				t.Fatalf("view exceeded %d columns: %s", size.Width, line)
			}
		}
	}
	update(tea.KeyMsg{Type: tea.KeyEnd})
	if update(tea.KeyMsg{Type: tea.KeyEnter}) != nil || m.chosen != nil || m.message == "" {
		t.Fatal("removed workspace was selectable")
	}
	update(tea.KeyMsg{Type: tea.KeyHome})
	if update(tea.KeyMsg{Type: tea.KeyEnter}) == nil || m.chosen == nil || m.chosen.Name != "one" {
		t.Fatal("Enter did not select the active workspace")
	}
	m.chosen = nil
	if update(tea.KeyMsg{Type: tea.KeyEsc}) == nil || m.chosen != nil {
		t.Fatal("cancel produced a selection")
	}
}

func TestHelpPresentationWrapsUsesThemeAndPreservesJSON(t *testing.T) {
	themeFixture(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	var out, logs bytes.Buffer
	s := New(core.New(t.TempDir(), t.TempDir(), &logs), &out, &logs)
	command, _, err := s.Root.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	command.InitDefaultHelpFlag()
	data, ok := helpData(command).(map[string]any)
	if !ok {
		t.Fatal("invalid help data")
	}
	for _, width := range []int{10, 34, 60, 80, 200} {
		p := humanPresentation(&out)
		p.Width = width
		text := helpPresentation(p, data)
		for _, line := range strings.Split(text, "\n") {
			if lipgloss.Width(line) > width-1 {
				t.Fatalf("help exceeded %d columns: %q", width, line)
			}
		}
	}
	if code := s.Execute(t.Context(), []string{"ls", "--help"}); code != 0 {
		t.Fatal(logs.String())
	}
	for _, want := range []string{"╭", "Flags", "--all", "switch", "38;2;187;154;247"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q: %s", want, out.String())
		}
	}
	result, code, _ := invoke(t, s.App, "sw", "--help", "--json")
	if code != 0 || result.Command != "wtf list" {
		t.Fatalf("JSON help changed: %+v", result)
	}
}

func TestBrowserSinglePageUsesHorizontalRowsWithoutPagination(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{Name: "one", Path: "/repo/one", Status: "ready"}, Exists: true},
		{Workspace: core.Workspace{Name: "two", Path: "/repo/two", Status: "ready"}, Exists: true},
	}
	m := newBrowser(rows, "/repo/one", humanPresentation(&bytes.Buffer{}), false, false)
	m.p.Width, m.height = 240, 60
	view := m.View()
	if !strings.HasPrefix(view, "╭─ Workspaces  2 ") {
		t.Fatalf("workspace caption is not in the top border: %s", view)
	}
	if strings.Contains(view, "1/1") || strings.Contains(view, "page") || strings.Contains(view, "LOCAL") || strings.Contains(view, "✓") {
		t.Fatalf("single page shows pagination: %s", view)
	}
	if lipgloss.Width(view) >= 120 || lipgloss.Height(view) > 7 {
		t.Fatalf("small list expands to fill terminal: %s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "two") && !strings.Contains(line, "../two") {
			t.Fatalf("workspace and path are stacked: %s", view)
		}
	}
}
