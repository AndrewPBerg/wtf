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

func TestRemovalPickerSharesMetadataBeforeIndependentSafetyInspections(t *testing.T) {
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "one", Name: "one", Owned: true}, Exists: true},
		{Workspace: core.Workspace{ID: "two", Name: "two", Owned: true}, Exists: true},
	}
	m := newRemovalPicker(removalPreview(rows, "/repo"), "/repo", humanPresentation(&bytes.Buffer{}))
	m.loading = true
	metadataCalls, inspections := 0, 0
	m.metadata = func() []core.WorkspaceView {
		metadataCalls++
		observed := append([]core.WorkspaceView{}, rows...)
		for i := range observed {
			observed[i].GitHub = &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{Number: 7}}}
		}
		return observed
	}
	m.inspect = func(row core.WorkspaceView) (core.RemovalCandidate, error) {
		inspections++
		if row.GitHub == nil || row.GitHub.Lookup != "fresh" {
			t.Fatal("safety inspection missed shared metadata")
		}
		return core.RemovalCandidate{WorkspaceView: row, Selectable: true}, nil
	}
	initial := m.Init()
	if metadataCalls != 0 || inspections != 0 {
		t.Fatal("initial UI waited for provider metadata")
	}
	next, cmd := m.Update(initial())
	m = next.(removalPicker)
	if metadataCalls != 1 || inspections != 0 || !m.loading {
		t.Fatal("metadata did not finish once before local inspection")
	}
	commands := cmd().(tea.BatchMsg)
	for _, inspect := range commands {
		next, _ := m.Update(inspect())
		m = next.(removalPicker)
	}
	if metadataCalls != 1 || inspections != 2 || m.loading || !m.rows[0].Selectable || !m.rows[1].Selectable {
		t.Fatal("shared metadata or independent safety assessments were lost")
	}
}

func TestRemovalPickerSelectsAcrossPagesAndRequiresConfirmation(t *testing.T) {
	rows := []core.RemovalCandidate{}
	for _, name := range []string{"one", "two", "ahead"} {
		rows = append(rows, core.RemovalCandidate{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: name, Name: name, Path: "/repo/" + name}}, Selectable: true, Integrated: name != "ahead", Base: "main", Ahead: 1})
	}
	rows = append(rows, core.RemovalCandidate{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "dirty", Name: "dirty", Path: "/repo/dirty"}}, Code: "workspace_has_changes", Warning: "Changes remain", Changes: []string{"M file"}})
	m := newRemovalPicker(rows, "/repo", humanPresentation(&bytes.Buffer{}))
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(msg)
		m = next.(removalPicker)
		return cmd
	}
	update(tea.WindowSizeMsg{Width: 100, Height: 10})
	update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if len(m.picked()) != 2 || m.selected["ahead"] || m.selected["dirty"] {
		t.Fatal("select-integrated included risky work")
	}
	update(tea.KeyMsg{Type: tea.KeyEnd})
	update(tea.KeyMsg{Type: tea.KeySpace})
	if m.selected["dirty"] || m.message == "" {
		t.Fatal("dirty workspace could be selected")
	}
	if cmd := update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || !m.confirming || len(m.chosen) != 0 {
		t.Fatal("first Enter removed without review")
	}
	update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirming || len(m.picked()) != 2 {
		t.Fatal("back discarded selection")
	}
	update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd := update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil || len(m.chosen) != 2 {
		t.Fatal("confirmation did not return selected identities")
	}
}

func TestRemovalPickerCancelWarningsAndSmallTerminals(t *testing.T) {
	row := core.RemovalCandidate{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "one", Name: "one", Path: "/repo/one"}}, Selectable: true, Warning: "Work is not verified in main/master"}
	m := newRemovalPicker([]core.RemovalCandidate{row}, "/repo", humanPresentation(&bytes.Buffer{}))
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 24}, {Width: 34, Height: 10}, {Width: 9, Height: 5}} {
		next, _ := m.Update(size)
		m = next.(removalPicker)
		if lipgloss.Height(m.View()) > size.Height {
			t.Fatal("picker exceeds terminal height")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > size.Width-1 {
				t.Fatal("picker exceeds terminal width")
			}
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(removalPicker)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(removalPicker)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(removalPicker)
	if cmd == nil || len(m.chosen) != 0 {
		t.Fatal("cancel confirmed removal")
	}
}

func TestRemovalJSONWithoutNameNeverPromptsOrDeletes(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	result, code, _ := invoke(t, a, "remove", "--json")
	if code == 0 || result.Error == nil || result.Error.Code != "not_a_jj_repository" {
		t.Fatalf("unexpected no-name JSON result: %+v", result)
	}
}

func TestRemovalForceToggleSelectionAndReview(t *testing.T) {
	rows := []core.RemovalCandidate{
		{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "dirty", Name: "dirty"}}, Code: "workspace_has_changes", Changes: []string{"M file"}},
		{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "protected", Name: "default"}}, Code: "protected_workspace"},
		{WorkspaceView: core.WorkspaceView{Workspace: core.Workspace{ID: "checking", Name: "checking"}}, Code: "checking"},
	}
	m := newRemovalPicker(rows, "/repo", humanPresentation(&bytes.Buffer{}))
	update := func(msg tea.Msg) tea.Cmd {
		next, cmd := m.Update(msg)
		m = next.(removalPicker)
		return cmd
	}
	force := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	update(tea.KeyMsg{Type: tea.KeySpace})
	if len(m.picked()) != 0 {
		t.Fatal("normal mode selected changes")
	}
	update(force)
	update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.force || len(m.picked()) != 1 || !strings.Contains(m.View(), "Force ON") {
		t.Fatal("force did not unlock changed row")
	}
	update(force)
	if len(m.picked()) != 0 || m.selected["dirty"] {
		t.Fatal("turning force off retained destructive selection")
	}
	update(force)
	update(tea.KeyMsg{Type: tea.KeySpace})
	for range 2 {
		update(tea.KeyMsg{Type: tea.KeyDown})
		update(tea.KeyMsg{Type: tea.KeySpace})
	}
	if len(m.picked()) != 1 {
		t.Fatal("force bypassed protected or pending row")
	}
	update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.confirming || !strings.Contains(m.View(), "discards remaining changes in 1") {
		t.Fatal("force review lacked destructive consequence")
	}
	update(force)
	if !m.force {
		t.Fatal("review changed force intent")
	}
	if update(tea.KeyMsg{Type: tea.KeyEnter}) == nil || len(m.chosen) != 1 {
		t.Fatal("force confirmation did not return chosen workspace")
	}
}

func TestRemovalLazyLoadingKeepsWorkspaceRowsAndRequiresSafetyResults(t *testing.T) {
	initial := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "default", Name: "default", Path: "/repo", Status: "ready"}, Exists: true},
		{Workspace: core.Workspace{ID: "feature", Name: "feature", Path: "/repo-feature", Status: "ready", Owned: true}, Exists: true},
	}
	m := newRemovalPicker(removalPreview(initial, "/repo"), "/repo", humanPresentation(&bytes.Buffer{}))
	m.loading = true
	calls := 0
	m.inspect = func(row core.WorkspaceView) (core.RemovalCandidate, error) {
		calls++
		if row.ID != initial[calls-1].ID {
			t.Fatal("inspection did not follow display order")
		}
		if row.ID == "default" {
			return core.RemovalCandidate{WorkspaceView: row, Code: "current_workspace", Warning: "Current workspace"}, nil
		}
		return core.RemovalCandidate{WorkspaceView: row, Integrated: true, Base: "main", Selectable: true}, nil
	}
	first := m.View()
	for _, name := range []string{"default", "feature", "../repo-feature", "MAIN", "CHANGES", "↻"} {
		if !strings.Contains(first, name) {
			t.Fatalf("initial view missing %q: %s", name, first)
		}
	}
	if strings.Contains(first, "Inspecting changes") || calls != 0 {
		t.Fatal("initial table waited for cleanup inspection")
	}
	update := func(msg tea.Msg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(msg)
		m = next.(removalPicker)
		return cmd
	}
	update(tea.KeyMsg{Type: tea.KeyDown})
	update(tea.KeyMsg{Type: tea.KeySpace})
	if update(tea.KeyMsg{Type: tea.KeyEnter}) != nil || m.confirming || len(m.picked()) != 0 {
		t.Fatal("removal enabled before safety checks completed")
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("no background inspection scheduled")
	}
	commands := cmd().(tea.BatchMsg)
	if len(commands) != 2 {
		t.Fatal("initial inspections were not dispatched together")
	}
	update(commands[0]())
	if !m.loading || calls != 1 || m.cursor != 1 {
		t.Fatal("first result did not publish independently")
	}
	update(commands[1]())
	if m.loading || calls != 2 || m.cursor != 1 {
		t.Fatal("completion lost the selected workspace identity")
	}
	if lipgloss.Height(m.View()) != lipgloss.Height(first) {
		t.Fatal("completion changed the table's vertical shape")
	}
	update(tea.KeyMsg{Type: tea.KeySpace})
	if len(m.picked()) != 1 {
		t.Fatal("verified workspace stayed disabled")
	}
}

func TestRemovalPickerReadyRowsSelectableWhileLaterRowsLoad(t *testing.T) {
	initial := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "one", Name: "one", Path: "/repo-one", Owned: true}, Exists: true},
		{Workspace: core.Workspace{ID: "two", Name: "two", Path: "/repo-two", Owned: true}, Exists: true},
	}
	m := newRemovalPicker(removalPreview(initial, "/repo"), "/repo", humanPresentation(&bytes.Buffer{}))
	m.loading = true
	m.inspect = func(row core.WorkspaceView) (core.RemovalCandidate, error) {
		return core.RemovalCandidate{WorkspaceView: row, Selectable: true, Integrated: true}, nil
	}
	commands := m.Init()().(tea.BatchMsg)
	next, _ := m.Update(commands[0]())
	m = next.(removalPicker)
	if !m.loading || !m.rows[0].Selectable || m.rows[1].Selectable {
		t.Fatal("results were not published incrementally")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(removalPicker)
	if len(m.picked()) != 1 {
		t.Fatal("ready row blocked by later inspection")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(removalPicker)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(removalPicker)
	if len(m.picked()) != 1 || m.message != "Checking cleanup safety…" {
		t.Fatal("pending row was selectable or lacked feedback")
	}
	next, _ = m.Update(commands[1]())
	m = next.(removalPicker)
	if m.loading || len(m.picked()) != 1 || !m.rows[1].Selectable {
		t.Fatal("later result lost selection")
	}
}

func TestRemovalPickerFanoutIsBoundedAndPublishesOutOfOrder(t *testing.T) {
	initial := []core.WorkspaceView{}
	for i := range removalInspectionWorkers + 3 {
		name := fmt.Sprintf("workspace-%02d", i)
		initial = append(initial, core.WorkspaceView{Workspace: core.Workspace{ID: name, Name: name, Path: "/repo-" + name, Owned: true}, Exists: true})
	}
	m := newRemovalPicker(removalPreview(initial, "/repo"), "/repo", humanPresentation(&bytes.Buffer{}))
	m.loading = true
	m.cursor = 7
	m.inspect = func(row core.WorkspaceView) (core.RemovalCandidate, error) {
		if row.ID == initial[0].ID {
			return core.RemovalCandidate{}, core.Fail("inspection_failed", "Unavailable")
		}
		return core.RemovalCandidate{WorkspaceView: row, Selectable: true}, nil
	}
	commands := m.Init()().(tea.BatchMsg)
	if len(commands) != removalInspectionWorkers {
		t.Fatalf("initial fan-out: %d", len(commands))
	}
	// Complete the bottom dispatched row before any earlier row.
	next, replacement := m.Update(commands[len(commands)-1]())
	m = next.(removalPicker)
	if !m.rows[31].Selectable || m.rows[0].Selectable || !m.loading || m.cursor != 7 || replacement == nil {
		t.Fatal("out-of-order result was blocked or lost cursor")
	}
	if replacement().(removalCandidatesMsg).id != initial[32].ID {
		t.Fatal("replacement was not dispatched in display order")
	}
	commands = append(commands[:len(commands)-1], replacement)
	completed := 1
	for len(commands) > 0 {
		cmd := commands[0]
		commands = commands[1:]
		next, replacement = m.Update(cmd())
		m = next.(removalPicker)
		completed++
		if replacement != nil {
			commands = append(commands, replacement)
		}
		if len(commands) > removalInspectionWorkers || m.loading != (completed < len(initial)) {
			t.Fatal("fan-out bound or loading count incorrect")
		}
	}
	if completed != len(initial) || m.rows[0].Code != "inspection_failed" || !m.rows[34].Selectable {
		t.Fatal("failed row prevented remaining inspections")
	}
}
