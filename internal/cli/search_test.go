package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func searchKey(value string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)} }

func TestPickerSearchFuzzyNamesMessagesAndUnicode(t *testing.T) {
	row := core.WorkspaceView{Workspace: core.Workspace{Name: "ALI-4380-corelita", Message: "Résumé booking privacy"}}
	for _, query := range []string{"a438", "RÉbP", "bkg", ""} {
		if !(pickerSearch{query: query}).matches(row) {
			t.Fatalf("no match for %q", query)
		}
	}
	if (pickerSearch{query: "xyz"}).matches(row) {
		t.Fatal("unrelated query matched")
	}
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	p := humanPresentation(&bytes.Buffer{})
	rendered := (pickerSearch{query: "réb"}).highlight(p, row.Message)
	if rendered == row.Message || ansi.Strip(rendered) != row.Message {
		t.Fatalf("highlight damaged text or omitted style: %q", rendered)
	}
}

func TestBrowserSearchRefreshAndNoMatches(t *testing.T) {
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "one", Name: "one", Message: "booking privacy", Path: "/one"}, Exists: true},
		{Workspace: core.Workspace{ID: "two", Name: "two", Path: "/two"}, Exists: true},
	}
	m := newBrowser(rows, "", humanPresentation(&bytes.Buffer{}), false, false)
	update := func(msg tea.Msg) tea.Cmd { next, cmd := m.Update(msg); m = next.(browser); return cmd }
	update(searchKey("/"))
	update(searchKey("bkp"))
	if len(m.rows) != 1 || m.rows[0].ID != "one" {
		t.Fatal("message fuzzy filter failed")
	}
	update(prRefreshMsg{rows: rows})
	if len(m.rows) != 1 || m.search.query != "bkp" {
		t.Fatal("refresh discarded filter")
	}
	if cmd := update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || m.chosen != nil {
		t.Fatal("applying query selected a workspace")
	}
	update(searchKey("/"))
	update(searchKey("xyz"))
	if len(m.rows) != 0 || !strings.Contains(m.View(), "No matching") {
		t.Fatal("missing empty result state")
	}
	update(tea.KeyMsg{Type: tea.KeyEnter})
	update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != nil {
		t.Fatal("empty search selected hidden workspace")
	}
	if cmd := update(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil || len(m.rows) != 2 {
		t.Fatal("escape did not clear filter")
	}
}

func TestRemovalSearchKeepsSelectionsAndReviewsAll(t *testing.T) {
	rows := []core.RemovalCandidate{
		{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "one", Name: "one", Message: "booking privacy"}}, Selectable: true, Integrated: true},
		{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "two", Name: "two", Message: "feature alpha"}}, Selectable: true, Integrated: true},
	}
	m := newRemovalPicker(rows, "", humanPresentation(&bytes.Buffer{}))
	update := func(msg tea.Msg) tea.Cmd { next, cmd := m.Update(msg); m = next.(removalPicker); return cmd }
	update(tea.KeyMsg{Type: tea.KeySpace})
	update(searchKey("/"))
	update(searchKey("fa"))
	if m.force || len(m.picked()) != 1 || len(m.visible()) != 1 {
		t.Fatal("typing invoked removal shortcuts")
	}
	update(tea.KeyMsg{Type: tea.KeyEnter})
	update(searchKey("a"))
	if len(m.picked()) != 2 {
		t.Fatal("filter lost selections")
	}
	update(removalCandidatesMsg{id: "two", row: rows[1]})
	if len(m.visible()) != 1 || m.visible()[0].ID != "two" {
		t.Fatal("inspection broke filter")
	}
	if cmd := update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || !m.confirming || len(m.visible()) != 2 || len(m.chosen) != 0 {
		t.Fatal("review hid selections or removed early")
	}
	update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirming || len(m.visible()) != 1 {
		t.Fatal("back lost filter")
	}
	update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.visible()) != 2 || len(m.picked()) != 2 {
		t.Fatal("clear lost selections")
	}
}

func TestPickerSearchEditingConsumesShortcuts(t *testing.T) {
	s := pickerSearch{}
	s.update(searchKey("/"))
	s.update(searchKey("é"))
	s.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if s.query != "" {
		t.Fatal("backspace did not remove Unicode rune")
	}
	s.update(searchKey("qfajkg"))
	s.update(tea.KeyMsg{Type: tea.KeySpace})
	if s.query != "qfajkg " || !s.editing {
		t.Fatal("typing was not preserved")
	}
	if s.update(tea.KeyMsg{Type: tea.KeyCtrlC}) {
		t.Fatal("search swallowed cancellation")
	}
	s.update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if s.query != "" {
		t.Fatal("ctrl+u did not clear input")
	}
}
