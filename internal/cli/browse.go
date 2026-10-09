package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	_ "github.com/AndrewPBerg/wtf/internal/terminal" // Initialize background defaults before Bubble Tea's startup query.
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

func terminalWriter(out io.Writer) bool {
	f, ok := out.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(f.Fd())
}

func (s *Session) browseInteractive() bool {
	return !s.JSON && term.IsTerminal(os.Stdin.Fd()) && (terminalWriter(s.Out) || (s.SelectPath && terminalWriter(s.Err)))
}

func (s *Session) browse(ctx context.Context, rows []core.WorkspaceView) (any, error) {
	out := s.Out
	if s.SelectPath {
		out = s.Err
	}
	m := newBrowser(rows, s.App.Cwd, humanPresentation(out), s.ShowPRs, s.ShowStages)
	result, err := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(out), tea.WithContext(ctx)).Run()
	if err != nil {
		return nil, core.Fail("browse_failed", err.Error())
	}
	selected, ok := result.(browser)
	if !ok {
		return nil, core.Fail("browse_failed", "Workspace browser returned an invalid result")
	}
	if selected.chosen == nil {
		s.Emitted = true // Cancellation produces no path, so shell integration cannot cd.
		return nil, nil
	}
	return map[string]string{"path": selected.chosen.Path, "id": selected.chosen.ID}, nil
}

type browser struct {
	rows                []core.WorkspaceView
	cwd                 string
	p                   presentation
	cursor, height      int
	showPRs, showStages bool
	chosen              *core.WorkspaceView
	message             string
}

func newBrowser(rows []core.WorkspaceView, cwd string, p presentation, prs, stages bool) browser {
	m := browser{rows: rows, cwd: cwd, p: p, height: 24, showPRs: prs, showStages: stages}
	for i, row := range rows {
		if row.Exists && row.Status != "removed" && currentPath(row.Path, cwd) {
			m.cursor = i
			break
		}
	}
	return m
}

func (m browser) Init() tea.Cmd { return nil }

func (m browser) pageSize() int { return max(1, min(10, m.height-7)) }

func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.p.Width, m.height = max(1, msg.Width), max(1, msg.Height)
	case tea.KeyMsg:
		m.message = ""
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "pgup", "left":
			m.cursor -= m.pageSize()
		case "pgdown", "right":
			m.cursor += m.pageSize()
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(m.rows) - 1
		case "enter":
			for i, row := range m.rows {
				if i != m.cursor {
					continue
				}
				if !row.Exists || row.Status == "removed" {
					m.message = "Workspace path is unavailable"
					return m, nil
				}
				m.chosen = &row
				return m, tea.Quit
			}
		}
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	}
	return m, nil
}

func (m browser) View() string {
	nameWidth, pathWidth, prWidth := 9, 4, 2
	for _, row := range m.rows {
		nameWidth = max(nameWidth, min(32, lipgloss.Width(plain(row.Name))))
		pathWidth = max(pathWidth, min(60, lipgloss.Width(listPath(row.Path, m.cwd))))
		if m.showPRs {
			prWidth = max(prWidth, min(24, lipgloss.Width(prLabel(row.GitHub))))
		}
	}
	desired := 4 + nameWidth + 2 + pathWidth + 4
	if m.showPRs {
		desired += prWidth + 2
	}
	if m.showStages {
		desired += 8
	}
	width := max(1, min(m.p.Width-1, max(48, desired)))
	inner := max(1, width-4)
	clip := func(s string) string { return ansi.Truncate(s, inner, "…") }
	cell := func(s string, cells int) string {
		s = ansi.Truncate(s, cells, "…")
		return s + strings.Repeat(" ", max(0, cells-lipgloss.Width(s)))
	}
	page := m.cursor / m.pageSize()
	pages := max(1, (len(m.rows)+m.pageSize()-1)/m.pageSize())
	title := fmt.Sprintf("Workspaces  %d", len(m.rows))
	if pages > 1 {
		title += fmt.Sprintf(" · %d/%d", page+1, pages)
	}
	lines := []string{m.p.Title.Render(clip(title))}
	header := "    " + cell("WORKSPACE", nameWidth) + "  "
	if m.showPRs {
		header += cell("PR", prWidth) + "  "
	}
	if m.showStages {
		header += "STAGES  "
	}
	lines = append(lines, m.p.Muted.Render(clip(header+"PATH")))
	if len(m.rows) == 0 {
		lines = append(lines, m.p.Muted.Render(clip("No active workspaces in this scope.")))
	}
	for i, row := range m.rows {
		if i < page*m.pageSize() || i >= (page+1)*m.pageSize() {
			continue
		}
		pointer, current := " ", " "
		if i == m.cursor {
			pointer = "›"
		}
		if row.Exists && row.Status != "removed" && currentPath(row.Path, m.cwd) {
			current = "●"
		}
		label := pointer + " " + current + " " + cell(plain(row.Name), nameWidth) + "  "
		if m.showPRs {
			label += cell(prLabel(row.GitHub), prWidth) + "  "
		}
		if m.showStages {
			passed := 0
			for _, stage := range row.Stages {
				if stage.Status == "succeeded" {
					passed++
				}
			}
			label += cell(fmt.Sprintf("%d/%d", passed, len(row.Stages)), 6) + "  "
		}
		style := m.p.Text
		if i == m.cursor {
			style = m.p.Title
		}
		lines = append(lines, clip(style.Render(label)+m.p.Muted.Render(listPath(row.Path, m.cwd))))
	}
	if m.message != "" {
		lines = append(lines, m.p.Bad.Render(clip(m.message)))
	} else {
		hint := "↑↓ move · Enter select · Esc close"
		if pages > 1 {
			hint = "↑↓ move · ←→ page · Enter select · Esc close"
		}
		lines = append(lines, m.p.Muted.Render(clip(hint)))
	}
	content := strings.Join(lines, "\n")
	if m.height < 7 || width < 12 {
		return ansi.Truncate(strings.Join(strings.Fields(ansi.Strip(content)), " "), width, "…")
	}
	return m.p.Renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.p.Muted.GetForeground()).Padding(0, 1).Width(inner + 2).Render(content)
}
