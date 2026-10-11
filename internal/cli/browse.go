package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
	browseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := s.Out
	if s.SelectPath {
		out = s.Err
	}
	m := newBrowser(rows, s.App.Cwd, humanPresentation(out), s.ShowPRs, s.ShowStages)
	m.showChecks = s.ShowChecks
	if s.ShowPRs || s.ShowChecks {
		service := s.App.GitHubService()
		m.rows = service.Prepare(browseCtx, rows, false).Rows
		m.fetchPRs = func() []core.WorkspaceView {
			plan := service.Prepare(browseCtx, rows, true)
			refreshed := service.Refresh(browseCtx, plan)
			if s.ShowPRs {
				refreshed = s.App.WithReviewLinks(browseCtx, refreshed, s.RefreshPRs)
			}
			return refreshed
		}
		m.refreshing = true
	}
	result, err := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(out), tea.WithContext(browseCtx)).Run()
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
	showChecks          bool
	chosen              *core.WorkspaceView
	message             string
	fetchPRs            func() []core.WorkspaceView
	refreshing          bool
}

type prRefreshMsg struct {
	rows []core.WorkspaceView
}

const prRefreshInterval = 15 * time.Second

type prPollMsg struct{}

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

func (m browser) Init() tea.Cmd { return m.refreshPRs() }

func (m browser) refreshPRs() tea.Cmd {
	if m.fetchPRs == nil {
		return nil
	}
	return func() tea.Msg { return prRefreshMsg{m.fetchPRs()} }
}

func (m browser) pageSize() int { return max(1, min(10, m.height-7)) }

func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prRefreshMsg:
		m.rows, m.refreshing = msg.rows, false
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		return m, tea.Tick(prRefreshInterval, func(time.Time) tea.Msg { return prPollMsg{} })
	case prPollMsg:
		if m.fetchPRs == nil || m.refreshing {
			return m, nil
		}
		m.refreshing = true
		return m, m.refreshPRs()
	case tea.WindowSizeMsg:
		m.p.Width, m.height = max(1, msg.Width), max(1, msg.Height)
	case tea.KeyMsg:
		m.message = ""
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.rows) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(m.rows) {
				m.cursor = 0
			}
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
	nameWidth, pathWidth, prWidth, ciWidth := 9, 4, 2, 2
	for _, row := range m.rows {
		nameWidth = max(nameWidth, min(32, lipgloss.Width(plain(row.Name))))
		pathWidth = max(pathWidth, min(60, lipgloss.Width(listPath(row.Path, m.cwd))))
		if m.showPRs {
			prWidth = max(prWidth, min(24, lipgloss.Width(prPresentation(m.p, row.GitHub))))
		}
		if m.showChecks {
			ciWidth = max(ciWidth, min(20, lipgloss.Width(ciPresentation(m.p, row.GitHub))))
		}
	}
	desired := 4 + nameWidth + 2 + pathWidth + 4
	if m.showPRs {
		desired += prWidth + 2
	}
	if m.showChecks {
		desired += ciWidth + 2
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
	if m.refreshing {
		title += " · ↻"
	}
	lines := []string{}
	header := "    " + cell("WORKSPACE", nameWidth) + "  "
	if m.showPRs {
		header += cell("PR", prWidth) + "  "
	}
	if m.showChecks {
		header += cell("CI", ciWidth) + "  "
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
			label += cell(prPresentation(m.p, row.GitHub), prWidth) + "  "
		}
		if m.showChecks {
			label += cell(ciPresentation(m.p, row.GitHub), ciWidth) + "  "
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
		return ansi.Truncate(title+" "+strings.Join(strings.Fields(ansi.Strip(content)), " "), width, "…")
	}
	return titledPanel(m.p, title, content, width)
}
