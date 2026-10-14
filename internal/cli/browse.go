package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
		service := s.App.Metadata()
		request := core.MetadataRequest{PRs: s.ShowPRs, CI: s.ShowChecks, Reviews: s.ShowPRs, ForceReviews: s.RefreshPRs}
		m.setRows(service.Prepare(browseCtx, rows, request).Cached)
		m.fetchPRs = func() []core.WorkspaceView {
			return service.Observe(browseCtx, rows, request)
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
	allRows             []core.WorkspaceView
	search              pickerSearch
	cwd                 string
	p                   presentation
	cursor, height      int
	motions             pickerMotions
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
	m := browser{cwd: cwd, p: p, height: 24, showPRs: prs, showStages: stages}
	m.setRows(rows)
	for i, row := range m.rows {
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

func (m browser) hasMessages() bool {
	for _, row := range m.rows {
		if row.Message != "" {
			return true
		}
	}
	return false
}

func (m browser) pageSize() int {
	reserved := 7
	if m.search.hint() != "" {
		reserved++
	}
	if m.hasMessages() {
		reserved++
	}
	return max(1, min(10, m.height-reserved))
}

// Group by the durable repository identity, never by workspace names or paths.
// Metadata refreshes may arrive in a different order; keep the selected workspace.
func (m *browser) setRows(rows []core.WorkspaceView) {
	var selected core.WorkspaceView
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		selected = m.rows[m.cursor]
	}
	m.allRows = append([]core.WorkspaceView(nil), rows...)
	m.rows = nil
	for _, row := range rows {
		if m.search.matches(row) {
			m.rows = append(m.rows, row)
		}
	}
	paths := map[string]string{}
	for _, row := range m.rows {
		paths[row.RepositoryID] = row.RepositoryPath
	}
	sort.SliceStable(m.rows, func(i, j int) bool {
		a, b := m.rows[i], m.rows[j]
		if a.RepositoryID == b.RepositoryID {
			if a.RepositoryID == "" {
				return false // Unassociated rows retain their supplied ordering.
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			if a.Path != b.Path {
				return a.Path < b.Path
			}
			return a.ID < b.ID
		}
		if paths[a.RepositoryID] != paths[b.RepositoryID] {
			return paths[a.RepositoryID] < paths[b.RepositoryID]
		}
		return a.RepositoryID < b.RepositoryID
	})
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	for i, row := range m.rows {
		if row.RepositoryID == selected.RepositoryID && row.ID == selected.ID && row.Path == selected.Path {
			m.cursor = i
			break
		}
	}
}

type browserPage struct {
	start, end           int
	repo, repos          int
	groupStart, groupEnd int
}

func (m browser) pages() ([]browserPage, int) {
	pages := []browserPage{}
	groups, active := 0, 0
	for start := 0; start < len(m.rows); {
		end := start + 1
		for end < len(m.rows) && m.rows[end].RepositoryID == m.rows[start].RepositoryID {
			end++
		}
		groups++
		for offset := start; offset < end; offset += m.pageSize() {
			page := browserPage{start: offset, end: min(end, offset+m.pageSize()), repo: groups, groupStart: start, groupEnd: end}
			if m.cursor >= page.start && m.cursor < page.end {
				active = len(pages)
			}
			pages = append(pages, page)
		}
		start = end
	}
	if len(pages) == 0 {
		pages = append(pages, browserPage{})
	}
	for i := range pages {
		pages[i].repos = groups
	}
	return pages, active
}

func (m browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prRefreshMsg:
		m.setRows(msg.rows)
		m.refreshing = false
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
		oldQuery := m.search.query
		if m.search.update(msg) {
			m.motions = pickerMotions{}
			if oldQuery != m.search.query {
				m.setRows(m.allRows)
				m.cursor = 0
			}
			return m, nil
		}
		pages, pageIndex := m.pages()
		page := pages[pageIndex]
		doubleG := msg.String() == "g" && m.motions.g
		if cursor, handled := m.motions.move(msg.String(), m.cursor, len(m.rows), m.pageSize()); handled {
			m.cursor = cursor
			if doubleG {
				m.cursor = page.start
			} else if msg.String() == "G" {
				m.cursor = max(0, page.end-1)
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.rows) - 1
			}
		case "down":
			m.cursor++
			if m.cursor >= len(m.rows) {
				m.cursor = 0
			}
		case "pgup", "left", "h", "H":
			previous := pages[max(0, pageIndex-1)]
			m.cursor = min(max(previous.start, previous.end-1), previous.start+m.cursor-page.start)
		case "pgdown", "right", "l", "L":
			next := pages[min(len(pages)-1, pageIndex+1)]
			m.cursor = min(max(next.start, next.end-1), next.start+m.cursor-page.start)
		case "tab":
			m.cursor = page.groupEnd
			if m.cursor >= len(m.rows) {
				m.cursor = 0
			}
		case "shift+tab":
			previous := page.groupStart - 1
			if previous < 0 {
				previous = len(m.rows) - 1
			}
			for previous > 0 && m.rows[previous-1].RepositoryID == m.rows[previous].RepositoryID {
				previous--
			}
			m.cursor = max(0, previous)
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(m.rows) - 1
		case "enter":
			for i, row := range m.rows {
				if i != m.cursor {
					continue
				}
				if row.Status == "removed" {
					m.message = "Workspace was removed"
					return m, nil
				}
				if !row.Exists {
					m.message = "Directory missing: " + listPath(row.Path, m.cwd)
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
	pages, pageIndex := m.pages()
	page := pages[pageIndex]
	hint := "↑↓/jk move · [N]j/k · gg/G · Enter select · Esc close"
	if len(pages) > 1 {
		hint = "↑↓/jk move · [N]j/k · gg/G · ←→/hl page · Enter select · Esc close"
	}
	if page.repos > 1 {
		hint = "↑↓/jk · [N]j/k · gg/G · ←→/hl page · Tab/S-Tab repo · Enter select · Esc close"
	}
	hint = "/ search · " + hint
	nameWidth, prWidth, ciWidth, ageWidth := 9, 2, 2, 3
	now := time.Now()
	for _, row := range m.rows[page.start:page.end] {
		ageWidth = max(ageWidth, len(workspaceAge(row.CreatedAt, now)))
		nameWidth = max(nameWidth, min(32, lipgloss.Width(plain(workspaceDisplayName(row)))))
		if m.showPRs {
			prWidth = max(prWidth, min(24, lipgloss.Width(prPresentation(m.p, row.GitHub))))
		}
		if m.showChecks {
			ciWidth = max(ciWidth, min(20, lipgloss.Width(ciPresentation(m.p, row.GitHub))))
		}
	}
	desired := 4 + nameWidth + 2 + ageWidth + 4
	if m.showPRs {
		desired += prWidth + 2
	}
	if m.showChecks {
		desired += ciWidth + 2
	}
	if m.showStages {
		desired += 8
	}
	width := max(1, min(m.p.Width-1, max(48, desired, lipgloss.Width(hint)+4)))
	inner := max(1, width-4)
	clip := func(s string) string { return ansi.Truncate(s, inner, "…") }
	cell := func(s string, cells int) string {
		s = ansi.Truncate(s, cells, "…")
		return s + strings.Repeat(" ", max(0, cells-lipgloss.Width(s)))
	}
	title := fmt.Sprintf("Workspaces  %d", len(m.rows))
	if page.repos > 1 {
		row := m.rows[page.start]
		name := filepath.Base(row.RepositoryPath)
		if row.RepositoryPath == "" {
			name = row.RepositoryID
		}
		title += fmt.Sprintf(" · %s (%d) · repo %d/%d", plain(name), page.groupEnd-page.groupStart, page.repo, page.repos)
	}
	if len(pages) > 1 {
		title += fmt.Sprintf(" · %d/%d", pageIndex+1, len(pages))
	}
	if m.refreshing {
		title += " · ↻"
	}
	if page.repos > 1 {
		width = min(m.p.Width-1, max(width, max(88, lipgloss.Width(title)+6)))
		width = max(1, width)
		inner = max(1, width-4)
	}
	lines := []string{}
	if hint := m.search.hint(); hint != "" {
		lines = append(lines, m.p.Title.Render(clip(hint)))
	}
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
	header += cell("AGE", ageWidth)
	lines = append(lines, m.p.Muted.Render(clip(header)))
	if len(m.rows) == 0 {
		empty := "No active workspaces in this scope."
		if m.search.query != "" {
			empty = "No matching workspaces."
		}
		lines = append(lines, m.p.Muted.Render(clip(empty)))
	}
	for i, row := range m.rows {
		if i < page.start || i >= page.end {
			continue
		}
		pointer, current := " ", " "
		if i == m.cursor {
			pointer = "›"
		}
		if row.Exists && row.Status != "removed" && currentPath(row.Path, m.cwd) {
			current = "●"
		}
		if !row.Exists || row.Status == "removed" {
			current = "!"
		}
		label := pointer + " " + current + " " + cell(workspaceSearchLink(m.p, row, m.search), nameWidth) + "  "
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
		label += cell(workspaceAge(row.CreatedAt, now), ageWidth)
		if i == m.cursor {
			style = m.p.Title
		}
		lines = append(lines, clip(style.Render(label)))
	}
	if m.hasMessages() {
		text := ""
		if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].Message != "" {
			text = "Message: " + m.search.highlight(m.p, m.rows[m.cursor].Message)
		}
		lines = append(lines, m.p.Text.Render(clip(text)))
	}
	if m.message != "" {
		lines = append(lines, m.p.Bad.Render(clip(m.message)))
	} else {
		lines = append(lines, m.p.Muted.Render(clip(hint)))
	}
	content := strings.Join(lines, "\n")
	if m.height < 7 || width < 12 {
		return ansi.Truncate(title+" "+strings.Join(strings.Fields(ansi.Strip(content)), " "), width, "…")
	}
	return titledPanel(m.p, title, content, width)
}
