package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestBrowserRepositoryPagesAndRefreshPreserveSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Interleave repositories as a path-sorted parent-directory listing does.
	rows := []core.WorkspaceView{}
	for i := range 5 {
		for _, repo := range []string{"next_gen_ui", "alita_core", "alita-mobile"} {
			rows = append(rows, core.WorkspaceView{
				Workspace:    core.Workspace{ID: fmt.Sprintf("%s-%d", repo, i), Name: fmt.Sprintf("task-%d", i), Path: fmt.Sprintf("/home/andrew/Desktop/Work/task-%d--%s", i, repo)},
				RepositoryID: repo, RepositoryPath: "/home/andrew/Desktop/Work/" + repo, Exists: true,
			})
		}
	}
	m := newBrowser(rows, "/home/andrew", humanPresentation(&bytes.Buffer{}), false, false)
	m.p.Width, m.height = 140, 10 // Three rows per page, two pages per repository.
	press := func(key tea.KeyMsg) {
		t.Helper()
		next, _ := m.Update(key)
		m = next.(browser)
	}
	pages, _ := m.pages()
	if len(pages) != 6 || !strings.Contains(m.View(), "alita-mobile (5) · repo 1/3 · 1/6") {
		t.Fatalf("unexpected repository pagination: %s", m.View())
	}
	for _, page := range pages {
		for _, row := range m.rows[page.start:page.end] {
			if row.RepositoryID != m.rows[page.start].RepositoryID {
				t.Fatal("page mixed repositories")
			}
		}
	}
	press(tea.KeyMsg{Type: tea.KeyRight})
	press(tea.KeyMsg{Type: tea.KeyRight})
	if m.cursor != 5 || !strings.Contains(m.View(), "alita_core (5) · repo 2/3 · 3/6") {
		t.Fatalf("partial repository page skipped the next repository: %s", m.View())
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.cursor != 7 {
		t.Fatal("G crossed a repository page boundary")
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.cursor != 5 {
		t.Fatal("gg crossed a repository page boundary")
	}
	press(tea.KeyMsg{Type: tea.KeyTab})
	if m.rows[m.cursor].RepositoryID != "next_gen_ui" {
		t.Fatal("Tab failed to jump repository")
	}
	press(tea.KeyMsg{Type: tea.KeyShiftTab})
	selected := m.rows[m.cursor]
	reversed := append([]core.WorkspaceView(nil), rows...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	next, _ := m.Update(prRefreshMsg{rows: reversed})
	m = next.(browser)
	if m.rows[m.cursor].ID != selected.ID {
		t.Fatal("refresh selected a different workspace")
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen == nil || m.chosen.ID != selected.ID {
		t.Fatal("Enter selected a different workspace after refresh")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 10}, {Width: 34, Height: 8}, {Width: 9, Height: 5}} {
		next, _ := m.Update(size)
		m = next.(browser)
		if lipgloss.Height(m.View()) > size.Height {
			t.Fatal("grouped picker exceeds terminal height")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > size.Width-1 {
				t.Fatal("grouped picker exceeds terminal width")
			}
		}
	}
}

func TestBrowserGroupsDistinctRepositoriesWithSameBasename(t *testing.T) {
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "a", Path: "/work/a"}, RepositoryID: "a", RepositoryPath: "/work/repo"},
		{Workspace: core.Workspace{ID: "b", Path: "/personal/b"}, RepositoryID: "b", RepositoryPath: "/personal/repo"},
	}
	m := newBrowser(rows, "/", humanPresentation(&bytes.Buffer{}), false, false)
	pages, _ := m.pages()
	if len(pages) != 2 || pages[0].repos != 2 {
		t.Fatal("same-named repositories were combined")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyTab}, {Type: tea.KeyShiftTab}} {
		empty := newBrowser(nil, "/", humanPresentation(&bytes.Buffer{}), false, false)
		next, _ := empty.Update(key)
		if next.(browser).cursor != 0 {
			t.Fatal("repository navigation moved an empty cursor")
		}
	}
}

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

func TestBrowserExplainsMissingDirectoryBeforeAndAfterSelection(t *testing.T) {
	rows := []core.WorkspaceView{{Workspace: core.Workspace{Name: "testing", Path: "/missing/testing", Status: "ready"}}}
	m := newBrowser(rows, "/repo", humanPresentation(&bytes.Buffer{}), false, false)
	if !strings.Contains(m.View(), "! testing") {
		t.Fatalf("missing workspace is not marked: %s", m.View())
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(browser)
	if cmd != nil || m.chosen != nil || m.message != "Directory missing: /missing/testing" {
		t.Fatalf("missing workspace selection: %+v", m)
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
	for _, want := range []string{"╭", "Flags", "--all", "ls", "38;2;187;154;247"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q: %s", want, out.String())
		}
	}
	result, code, _ := invoke(t, s.App, "ls", "--help", "--json")
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
		if strings.Contains(line, "two") && !strings.Contains(line, "?") {
			t.Fatalf("workspace and age are stacked: %s", view)
		}
	}
	if strings.Contains(view, "PATH") || strings.Contains(view, "../two") {
		t.Fatalf("path duplicates the workspace name: %s", view)
	}
	if !strings.Contains(view, "↑↓/jk move · [N]j/k · gg/G · Enter select · Esc close") {
		t.Fatalf("small list clips the navigation footer despite available terminal space: %s", view)
	}
}

func TestBrowserDoesNotClipPaddingAfterAge(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	row := core.WorkspaceView{
		Workspace: core.Workspace{Name: "booking-contact-details", Path: "/repo/booking-contact-details", Status: "ready"},
		Exists:    true,
		GitHub: &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{
			Number: 1615, State: "CLOSED", Head: "published", Checks: &core.CheckSummary{Head: "published", Passed: 3},
		}}},
	}
	m := newBrowser([]core.WorkspaceView{row}, "/repo", humanPresentation(&bytes.Buffer{}), true, false)
	m.showChecks = true
	m.p.Width = 120
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, row.Name) && !strings.HasSuffix(strings.Trim(line, " │"), "?") {
			t.Fatalf("row clipped padding after age:\n%s", view)
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
