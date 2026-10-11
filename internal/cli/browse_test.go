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
	m.showChecks = true
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

func TestBrowserArrowKeysWrapAcrossPagesAndHandleEmptyLists(t *testing.T) {
	rows := []core.WorkspaceView{}
	for _, name := range []string{"first", "middle", "last"} {
		rows = append(rows, core.WorkspaceView{Workspace: core.Workspace{Name: name, Path: "/repo/" + name}, Exists: true})
	}
	m := newBrowser(rows, "/repo/first", humanPresentation(&bytes.Buffer{}), false, false)
	m.height = 8 // One workspace per page.
	for _, key := range []tea.KeyMsg{{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyRunes, Runes: []rune{'k'}}, {Type: tea.KeyRunes, Runes: []rune{'j'}}} {
		next, _ := m.Update(key)
		m = next.(browser)
		want := 0
		if key.String() == "up" || key.String() == "k" {
			want = 2
		}
		if m.cursor != want {
			t.Fatalf("%s did not wrap: cursor %d", key.String(), m.cursor)
		}
	}
	empty := newBrowser(nil, "/repo", humanPresentation(&bytes.Buffer{}), false, false)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyUp}, {Type: tea.KeyDown}} {
		next, _ := empty.Update(key)
		empty = next.(browser)
		if empty.cursor != 0 {
			t.Fatal("empty browser moved its cursor")
		}
	}
}

func TestBrowserRefreshesRecentDraftCacheAndAutomaticallyRechecks(t *testing.T) {
	cached := []core.WorkspaceView{{Workspace: core.Workspace{ID: "workspace", Name: "pilot", Path: "/repo", Status: "ready"}, Exists: true,
		GitHub: &core.PRInfo{Lookup: "cached", Matches: []core.PullRequest{{Number: 19, State: "OPEN", Draft: true}}}}}
	fresh := []core.WorkspaceView{{Workspace: cached[0].Workspace, Exists: true,
		GitHub: &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{Number: 19, State: "OPEN"}}}}}
	m := newBrowser(cached, "/repo", humanPresentation(&bytes.Buffer{}), true, false)
	calls := 0
	m.fetchPRs = func() []core.WorkspaceView { calls++; return fresh }
	m.refreshing = true
	if !strings.Contains(m.View(), "draft") || !strings.Contains(m.View(), "↻") {
		t.Fatal("cached view did not indicate an active lookup")
	}
	initial := m.Init()
	if initial == nil {
		t.Fatal("recent cache suppressed the initial refresh")
	}
	next, followup := m.Update(initial())
	m = next.(browser)
	if calls != 1 || m.refreshing || followup == nil || strings.Contains(m.View(), "draft") || !strings.Contains(m.View(), "#19 open") {
		t.Fatalf("live refresh did not replace the recent cached draft: %s", m.View())
	}
	next, automatic := m.Update(prPollMsg{})
	m = next.(browser)
	if automatic == nil || !m.refreshing {
		t.Fatal("automatic refresh did not start")
	}
	if _, duplicate := m.Update(prPollMsg{}); duplicate != nil {
		t.Fatal("automatic refresh overlapped an existing lookup")
	}
	next, followup = m.Update(automatic())
	m = next.(browser)
	if calls != 2 || m.refreshing || m.cursor != 0 || followup == nil {
		t.Fatalf("unexpected refresh result: calls=%d cursor=%d refreshing=%v", calls, m.cursor, m.refreshing)
	}
	if strings.Contains(m.View(), "r refresh") {
		t.Fatal("automatic browser shows a manual refresh hint")
	}
}
