package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type removalFailure struct {
	Name    string `json:"name"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type removalResult struct {
	Removed []core.Workspace `json:"removed"`
	Failed  []removalFailure `json:"failed"`
}

func (s *Session) remove(ctx context.Context, args []string) (any, error) {
	if len(args) == 1 {
		for _, name := range args {
			return s.App.Remove(ctx, name)
		}
	}
	if !s.browseInteractive() {
		return s.App.RemovalCandidates(ctx)
	}
	pickerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	rows, err := s.App.List(pickerCtx, false, false)
	if err != nil {
		return nil, err
	}
	inspector, err := s.App.RemovalInspector(pickerCtx)
	if err != nil {
		return nil, err
	}
	m := newRemovalPicker(removalPreview(rows, s.App.Cwd), s.App.Cwd, humanPresentation(s.Out))
	m.loading = true
	for _, row := range rows {
		if row.Owned && row.Exists && !currentPath(row.Path, s.App.Cwd) {
			m.metadata = func() []core.WorkspaceView {
				return s.App.RemovalMetadata(pickerCtx, rows)
			}
			break
		}
	}
	m.inspect = func(row core.WorkspaceView) (core.RemovalCandidate, error) {
		return inspector.Inspect(pickerCtx, row)
	}
	model, err := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(s.Out), tea.WithContext(pickerCtx)).Run()
	if err != nil {
		return nil, core.Fail("removal_picker_failed", err.Error())
	}
	result, ok := model.(removalPicker)
	if !ok {
		return nil, core.Fail("removal_picker_failed", "Removal picker returned an invalid result")
	}
	if result.err != nil {
		return nil, result.err
	}
	if len(result.chosen) == 0 {
		s.Emitted = true
		return nil, nil
	}
	removed := removalResult{Removed: []core.Workspace{}, Failed: []removalFailure{}}
	for _, candidate := range result.chosen {
		workspace, removeErr := s.App.RemoveWithOptions(ctx, candidate.ID, candidate.RemovalOptions(result.force))
		if removeErr != nil {
			removed.Failed = append(removed.Failed, removalFailure{candidate.Name, core.ErrorCode(removeErr), removeErr.Error()})
		} else {
			removed.Removed = append(removed.Removed, workspace)
		}
		if ctx.Err() != nil {
			return removed, core.Fail("command_cancelled", ctx.Err().Error())
		}
	}
	if len(removed.Failed) > 0 {
		return removed, core.Fail("removal_incomplete", "Some workspaces could not be removed")
	}
	return removed, nil
}

func removalPreview(rows []core.WorkspaceView, cwd string) []core.RemovalCandidate {
	preview := []core.RemovalCandidate{}
	for _, row := range rows {
		if row.Status == "removed" || !row.Exists {
			continue
		}
		candidate := core.RemovalCandidate{WorkspaceView: row, Changes: []string{}, Code: "checking", Warning: "Checking cleanup safety…"}
		switch {
		case currentPath(row.Path, cwd):
			candidate.Code, candidate.Warning = "current_workspace", "Current workspace"
		case !row.Owned:
			candidate.Code, candidate.Warning = "protected_workspace", "Adopted or anchor workspace"
		case !row.Exists:
			candidate.Code, candidate.Warning = "workspace_unavailable", "Workspace path is unavailable"
		}
		preview = append(preview, candidate)
	}
	return preview
}

type removalCandidatesMsg struct {
	row core.RemovalCandidate
	id  string
	err error
}

type removalMetadataMsg struct{ rows []core.WorkspaceView }

const removalInspectionWorkers = 32

type removalPicker struct {
	rows                []core.RemovalCandidate
	cwd                 string
	p                   presentation
	cursor, height      int
	motions             pickerMotions
	selected            map[string]bool
	confirming, loading bool
	force               bool
	chosen              []core.RemovalCandidate
	message             string
	err                 error
	inspect             func(core.WorkspaceView) (core.RemovalCandidate, error)
	metadata            func() []core.WorkspaceView
	nextInspection      int
}

func newRemovalPicker(rows []core.RemovalCandidate, cwd string, p presentation) removalPicker {
	m := removalPicker{rows: rows, cwd: cwd, p: p, height: 24, selected: map[string]bool{}}
	for i, row := range rows {
		if currentPath(row.Path, cwd) {
			m.cursor = i
			break
		}
	}
	return m
}

func (m removalPicker) Init() tea.Cmd {
	if m.metadata != nil {
		return func() tea.Msg { return removalMetadataMsg{rows: m.metadata()} }
	}
	commands := []tea.Cmd{}
	for i := 0; i < min(removalInspectionWorkers, len(m.rows)); i++ {
		commands = append(commands, m.inspectRow(i))
	}
	return tea.Batch(commands...)
}

func (m removalPicker) inspectRow(index int) tea.Cmd {
	if m.inspect == nil || index >= len(m.rows) {
		return nil
	}
	row := m.rows[index].WorkspaceView
	return func() tea.Msg {
		candidate, err := m.inspect(row)
		return removalCandidatesMsg{row: candidate, id: row.ID, err: err}
	}
}

func (m removalPicker) picked() []core.RemovalCandidate {
	rows := []core.RemovalCandidate{}
	for _, row := range m.rows {
		if m.selected[row.ID] && row.CanRemove(m.force) {
			rows = append(rows, row)
		}
	}
	return rows
}

func (m removalPicker) visible() []core.RemovalCandidate {
	if m.confirming {
		return m.picked()
	}
	return m.rows
}

func (m removalPicker) hasMessages() bool {
	for _, row := range m.rows {
		if row.Message != "" {
			return true
		}
	}
	return false
}

func (m removalPicker) pageSize() int {
	reserved := 9
	if m.hasMessages() {
		reserved++
	}
	return max(1, min(10, m.height-reserved))
}

func (m removalPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case removalMetadataMsg:
		observed := make(map[string]core.WorkspaceView, len(msg.rows))
		for _, row := range msg.rows {
			observed[row.ID] = row
		}
		for i := range m.rows {
			if row, ok := observed[m.rows[i].ID]; ok {
				m.rows[i].WorkspaceView = row
			}
		}
		m.metadata = nil
		return m, m.Init()
	case removalCandidatesMsg:
		for i, row := range m.rows {
			if row.ID == msg.id {
				if !m.confirming && i == m.cursor && m.message == row.Warning {
					m.message = ""
				}
				if msg.err != nil {
					row.Code, row.Warning, row.Selectable = core.ErrorCode(msg.err), msg.err.Error(), false
					m.rows[i] = row
				} else {
					m.rows[i] = msg.row
				}
				break
			}
		}
		m.nextInspection++
		m.loading = m.nextInspection < len(m.rows)
		return m, m.inspectRow(m.nextInspection + removalInspectionWorkers - 1)
	case tea.WindowSizeMsg:
		m.p.Width, m.height = max(1, msg.Width), max(1, msg.Height)
	case tea.KeyMsg:
		m.message = ""
		if cursor, handled := m.motions.move(msg.String(), m.cursor, len(m.visible()), m.pageSize()); handled {
			if m.err == nil {
				m.cursor = cursor
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.confirming {
				m.confirming, m.cursor = false, 0
				return m, nil
			}
			return m, tea.Quit
		}
		if m.err != nil {
			return m, nil
		}
		rows := m.visible()
		switch msg.String() {
		case "up":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(rows) - 1
			}
		case "f", "F":
			if !m.confirming {
				m.force = !m.force
				if !m.force {
					for _, row := range m.rows {
						if !row.CanRemove(false) {
							delete(m.selected, row.ID)
						}
					}
					m.message = "Force OFF; selections requiring force cleared"
				} else {
					m.message = "Force ON; changed workspaces can be selected for removal"
				}
			}
		case "down":
			m.cursor++
			if m.cursor >= len(rows) {
				m.cursor = 0
			}
		case "left", "pgup", "h", "H":
			m.cursor -= m.pageSize()
		case "right", "pgdown", "l", "L":
			m.cursor += m.pageSize()
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(rows) - 1
		case " ":
			if !m.confirming {
				for i, row := range rows {
					if i == m.cursor {
						if !row.CanRemove(m.force) {
							m.message = row.Warning
							break
						}
						m.selected[row.ID] = !m.selected[row.ID]
					}
				}
			}
		case "a":
			if !m.confirming {
				all, count := true, 0
				for _, row := range rows {
					if row.Selectable && row.Integrated {
						count++
						all = all && m.selected[row.ID]
					}
				}
				for _, row := range rows {
					if row.Selectable && row.Integrated {
						m.selected[row.ID] = !all
					}
				}
				if count == 0 {
					m.message = "No clean workspaces verified in main/master"
				}
			}
		case "enter":
			picked := m.picked()
			if len(picked) == 0 {
				m.message = "Select workspaces with Space"
				return m, nil
			}
			if m.confirming {
				m.chosen = picked
				return m, tea.Quit
			}
			m.confirming, m.cursor = true, 0
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.visible())-1))
	return m, nil
}

func removalStatus(row core.RemovalCandidate) string {
	switch {
	case row.Code == "checking":
		return "↻"
	case row.Code == "current_workspace":
		return "●"
	case row.Code == "protected_workspace":
		return "⊘"
	case row.Integrated:
		return "✓"
	case row.Ahead > 0:
		return fmt.Sprintf("↑ %d", row.Ahead)
	default:
		return "?"
	}
}

func (m removalPicker) View() string {
	rows := m.visible()
	page, size := m.cursor/m.pageSize(), m.pageSize()
	pages := max(1, (len(rows)+size-1)/size)
	title := fmt.Sprintf("Remove workspaces  %d · %d selected", len(rows), len(m.picked()))
	if m.confirming {
		title = fmt.Sprintf("Confirm removal  %d", len(rows))
	}
	if m.loading {
		title += " · ↻"
	}
	if m.force {
		title += " · Force ON"
	} else {
		title += " · Force OFF"
	}
	if pages > 1 {
		title += fmt.Sprintf(" · %d/%d", page+1, pages)
	}
	nameWidth, statusWidth, changesWidth, ageWidth := 9, 4, 7, 3
	now := time.Now()
	for _, row := range rows {
		ageWidth = max(ageWidth, len(workspaceAge(row.CreatedAt, now)))
		nameWidth = max(nameWidth, min(32, lipgloss.Width(plain(workspaceDisplayName(row.WorkspaceView)))))
		statusWidth = max(statusWidth, min(28, lipgloss.Width(removalStatus(row))))
		if len(row.Changes) > 0 {
			changesWidth = max(changesWidth, lipgloss.Width(fmt.Sprintf("! %d", len(row.Changes))))
		}
	}
	// Six prefix cells, six separators and four frame cells surround the columns.
	width := max(1, min(m.p.Width-1, max(64, 16+nameWidth+statusWidth+changesWidth+ageWidth)))
	inner := max(1, width-4)
	clip := func(value string) string { return ansi.Truncate(value, inner, "…") }
	cell := func(value string, n int) string {
		value = ansi.Truncate(value, n, "…")
		return value + strings.Repeat(" ", max(0, n-lipgloss.Width(value)))
	}
	lines := []string{m.p.Muted.Render(clip("      " + cell("WORKSPACE", nameWidth) + "  " + cell("MAIN", statusWidth) + "  " + cell("CHANGES", changesWidth) + "  " + cell("AGE", ageWidth)))}
	if !m.loading && len(rows) == 0 {
		lines = append(lines, m.p.Muted.Render(clip("No active workspaces in this repository.")))
	}
	for i, row := range rows {
		if i < page*size || i >= (page+1)*size {
			continue
		}
		pointer, mark := " ", "[ ]"
		if i == m.cursor {
			pointer = "›"
		}
		if m.selected[row.ID] {
			mark = "[x]"
		}
		if !row.CanRemove(m.force) && row.Code != "checking" {
			mark = "[·]"
		}
		style := m.p.Warning
		if row.Integrated {
			style = m.p.Good
		}
		if !row.CanRemove(m.force) {
			style = m.p.Muted
		}
		if len(row.Changes) > 0 {
			style = m.p.Bad
		}
		changes := ""
		if row.Code == "checking" {
			changes = "…"
		}
		if len(row.Changes) > 0 {
			changes = fmt.Sprintf("! %d", len(row.Changes))
		}
		label := pointer + " " + mark + " " + cell(workspaceLinkPresentation(m.p, row.WorkspaceView, workspaceDisplayName(row.WorkspaceView)), nameWidth) + "  " + cell(removalStatus(row), statusWidth) + "  " + cell(changes, changesWidth) + "  " + cell(workspaceAge(row.CreatedAt, now), ageWidth)
		lines = append(lines, style.Render(clip(label)))
	}
	if m.hasMessages() {
		text := ""
		if m.cursor >= 0 && m.cursor < len(rows) && rows[m.cursor].Message != "" {
			text = "Message: " + plain(rows[m.cursor].Message)
		}
		lines = append(lines, m.p.Text.Render(clip(text)))
	}
	detail := m.message
	if detail == "" {
		for i, row := range rows {
			if i != m.cursor {
				continue
			}
			detail = row.Warning
			if row.Integrated && row.Base != "" {
				detail = "Compared with " + row.Base + " (local)"
			}
			if row.Integrated && strings.HasSuffix(row.Base, "@origin") {
				detail = "Compared with " + row.Base + " (last fetched)"
			}
			if strings.HasPrefix(row.Evidence, "merged_pr:") {
				detail = "Verified by merged PR #" + strings.TrimPrefix(row.Evidence, "merged_pr:") + " into " + row.Base
			}
			if len(row.Changes) > 0 {
				detail = fmt.Sprintf("%d changed files. %s", len(row.Changes), row.Warning)
			}
		}
	}
	if m.confirming {
		detail = "Removes selected directories and their owned resources."
		for _, row := range rows {
			if !row.Integrated {
				detail = "Warning: selected work is not verified in main/master."
				break
			}
		}
	}
	if m.confirming && m.force {
		changed := 0
		for _, row := range rows {
			if row.Code == "workspace_has_changes" {
				changed++
			}
		}
		detail = fmt.Sprintf("Force removal: discards remaining changes in %d workspace(s).", changed)
	}
	if detail != "" {
		lines = append(lines, m.p.Warning.Render(clip(plain(detail))))
	}
	hint := "↑↓/jk · Space toggle · f force · a select integrated"
	if m.confirming {
		hint = "Enter remove selected · Esc back · Ctrl+C cancel"
	}
	if pages > 1 {
		hint = "←→/hl page · " + hint
	}
	lines = append(lines, m.p.Muted.Render(clip(hint)))
	if !m.confirming {
		lines = append(lines, m.p.Muted.Render(clip("Enter review · Esc close")))
	}
	if m.height < 8 || width < 12 {
		return ansi.Truncate(title+" "+strings.Join(strings.Fields(ansi.Strip(strings.Join(lines, " "))), " "), width, "…")
	}
	return titledPanel(m.p, title, strings.Join(lines, "\n"), width)
}

func removalReport(p presentation, result removalResult) string {
	lines := []string{}
	for _, row := range result.Removed {
		lines = append(lines, p.Good.Render("✓ Removed ")+p.Text.Render(plain(row.Name))+"  "+p.Muted.Render(shortPath(row.Path)))
	}
	for _, failed := range result.Failed {
		lines = append(lines, p.Bad.Render("✗ Kept ")+p.Text.Render(plain(failed.Name))+"  "+p.Warning.Render(plain(failed.Message)))
	}
	return strings.Join(lines, "\n")
}
