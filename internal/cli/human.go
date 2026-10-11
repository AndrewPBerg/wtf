package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
)

func (s *Session) human(out io.Writer, err error) error {
	p := humanPresentation(out)
	data := s.Data
	if initialized, ok := data.(core.Initialization); ok {
		data = initialized.Workspace
	}
	if err != nil {
		if _, e := fmt.Fprintln(out, p.Bad.Bold(true).Render("Error")+"  "+plain(err.Error())); e != nil {
			return e
		}
		if w, ok := data.(core.Workspace); ok && w.ID != "" {
			_, e := fmt.Fprintln(out, p.Muted.Render("Workspace: ")+plain(w.Path))
			return e
		}
		if h, ok := s.Data.(core.Health); ok {
			_, e := fmt.Fprintln(out, doctorHealthText(p, h))
			return e
		}
		if result, ok := s.Data.(removalResult); ok {
			_, e := fmt.Fprintln(out, removalReport(p, result))
			return e
		}
		return nil
	}
	text := ""
	switch data := data.(type) {
	case core.SetupTeardownResult:
		switch data.Status {
		case "removed":
			text = p.Good.Render("✓ WTF configuration removed")
		case "not_configured":
			text = p.Muted.Render("No .wtf directory to remove.")
		default:
			text = p.Muted.Render("Teardown cancelled; .wtf preserved.")
		}
		text += "\n" + p.Muted.Render(shortPath(data.Path))
	case core.SetupResult:
		text = p.Good.Render("✓ WTF configuration ready") + "\n" + p.Muted.Render(shortPath(data.Path))
		for _, path := range data.Created {
			text += "\n  " + p.Good.Render("Created") + "  " + p.Text.Render(plain(path))
		}
		for _, path := range data.Existing {
			text += "\n  " + p.Muted.Render("Preserved") + "  " + p.Text.Render(plain(path))
		}
	case removalResult:
		text = removalReport(p, data)
	case []core.RemovalCandidate:
		text = newRemovalPicker(data, s.App.Cwd, p).View() + "\n" + p.Muted.Render("Use wtf remove in a terminal to select, or wtf remove NAME.")
	case []core.WorkspaceView:
		text = workspaceTable(p, data, s.App.Cwd, s.ShowStages, s.ShowPRs, s.ShowChecks)
	case core.Workspace:
		verb := "Workspace ready"
		if data.Status == "removed" {
			verb = "Workspace removed"
		}
		text = p.Good.Render("✓ "+verb) + "  " + p.Title.Render(plain(data.Name)) + "\n" + p.Muted.Render(shortPath(data.Path))
		names := make([]string, 0, len(data.Stages))
		for name := range data.Stages {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stage := data.Stages[name]
			detail := stage.Status
			if stage.Reused {
				detail += " · reused"
			}
			text += "\n  " + p.Muted.Render(plain(name)) + "  " + p.Good.Render(detail)
		}
	case core.Stage:
		text = p.Good.Render("✓ Task " + data.Status)
		if data.Reused {
			text += p.Muted.Render(" · reused")
		}
	case core.Health:
		text = doctorHealthText(p, data)
	case core.Config:
		text = p.Good.Render("✓ Configuration valid")
		if data.Lua != "" {
			text += "\n" + p.Muted.Render("Hooks: "+plain(data.Lua))
		}
		names := make([]string, 0, len(data.Tasks))
		for name := range data.Tasks {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > 0 {
			text += "\n" + p.Muted.Render("Tasks: "+plain(strings.Join(names, ", ")))
		}
	case []core.Claim:
		if len(data) == 0 {
			text = p.Muted.Render("No port claims in this workspace.")
		} else {
			lines := []string{p.Title.Render("Ports")}
			for _, c := range data {
				lines = append(lines, p.Text.Render(plain(c.Name))+"  "+p.Muted.Render(plain(c.Value)))
			}
			text = strings.Join(lines, "\n")
		}
	case core.Claim:
		text = p.Good.Render("✓ Claimed ") + p.Text.Render(plain(data.Name)) + "  " + p.Muted.Render(plain(data.Value))
	case map[string][]string:
		lines := []string{p.Title.Render("Hooks")}
		for _, group := range []string{"hooks", "overrides"} {
			for _, name := range data[group] {
				lines = append(lines, "  "+p.Text.Render(plain(name))+"  "+p.Muted.Render(group))
			}
		}
		if len(lines) == 1 {
			lines = append(lines, p.Muted.Render("No hooks configured."))
		}
		text = strings.Join(lines, "\n")
	case map[string]string:
		if path, ok := data["path"]; ok {
			if s.SelectPath {
				text = path
			} else {
				text = p.Title.Render("→ ") + p.Text.Render(shortPath(path))
			}
		} else if version, ok := data["version"]; ok {
			text = p.Title.Render("WTF ") + plain(version)
		} else if name, ok := data["released"]; ok {
			text = p.Good.Render("✓ Released ") + plain(name)
		} else {
			text = humanFields(p, data)
		}
	case map[string]any:
		if _, ok := data["usage"].(string); ok {
			text = helpPresentation(p, data)
		} else if usage, ok := data["text"].(string); ok {
			text = usage
		} else {
			text = p.Good.Render("✓ Command completed")
		}
	default:
		text = p.Good.Render("✓ Command completed")
	}
	_, e := fmt.Fprintln(out, text)
	return e
}

func humanFields(p presentation, data map[string]string) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, key := range keys {
		lines = append(lines, p.Muted.Render(plain(key))+"  "+p.Text.Render(plain(data[key])))
	}
	return strings.Join(lines, "\n")
}
func shortPath(path string) string {
	home, e := os.UserHomeDir()
	if e == nil {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(os.PathSeparator)) {
			return plain("~" + strings.TrimPrefix(path, home))
		}
	}
	return plain(path)
}
func currentPath(path, cwd string) bool {
	rel, e := filepath.Rel(path, cwd)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func listPath(path, cwd string) string {
	rel, err := filepath.Rel(cwd, path)
	if err == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.Dir(rel) == "..") {
		return plain(rel)
	}
	return shortPath(path)
}

func workspaceTable(p presentation, workspaces []core.WorkspaceView, cwd string, showStages, showPRs bool, checks ...bool) string {
	showChecks := false
	for _, enabled := range checks {
		showChecks = showChecks || enabled
	}
	if len(workspaces) == 0 {
		return p.Title.Render("Workspaces") + "\n" + p.Muted.Render("No active workspaces in this scope.")
	}
	rows := [][]string{}
	states := []string{}
	headers := []string{"", "WORKSPACE"}
	widths := []int{3, 11}
	if showStages {
		headers = append(headers, "STAGES")
		widths = append(widths, 8)
	}
	prColumn := -1
	if showPRs {
		prColumn = len(headers)
		headers = append(headers, "PR")
		widths = append(widths, 14)
	}
	ciColumn := -1
	if showChecks {
		ciColumn = len(headers)
		headers = append(headers, "CI")
		widths = append(widths, 6)
	}
	ageColumn := len(headers)
	headers = append(headers, "AGE")
	widths = append(widths, 5)
	now := time.Now()
	for _, w := range workspaces {
		marker := ""
		if w.Exists && w.Status != "removed" && currentPath(w.Path, cwd) {
			marker = "●"
		}
		status := strings.ReplaceAll(w.Status, "_", " ")
		if !w.Exists && w.Status != "removed" {
			status = "missing"
		}
		passed := 0
		for _, st := range w.Stages {
			if st.Status == "succeeded" {
				passed++
			}
		}
		stages := "—"
		if len(w.Stages) > 0 {
			stages = fmt.Sprintf("%d/%d", passed, len(w.Stages))
		}
		row := []string{marker, workspaceLinkPresentation(p, w, w.Name)}
		if showStages {
			row = append(row, stages)
		}
		if showPRs {
			row = append(row, prPresentation(p, w.GitHub))
		}
		if showChecks {
			row = append(row, ciPresentation(p, w.GitHub))
		}
		rows = append(rows, append(row, workspaceAge(w.CreatedAt, now)))
		states = append(states, status)
	}
	width := min(p.Width-1, 140) // Reserve a cell to avoid terminal-edge wrapping.
	title := p.Title.Render("Workspaces") + p.Muted.Render(fmt.Sprintf("  %d", len(workspaces)))
	for _, row := range rows {
		widths[1] = max(widths[1], min(lipgloss.Width(row[1])+2, 24))
		if showPRs {
			widths[prColumn] = max(widths[prColumn], min(lipgloss.Width(row[prColumn])+2, 24))
		}
		if showChecks {
			widths[ciColumn] = max(widths[ciColumn], min(lipgloss.Width(row[ciColumn])+2, 22))
		}
		widths[ageColumn] = max(widths[ageColumn], lipgloss.Width(row[ageColumn])+2)
	}
	borders := len(headers) + 1
	tableWidth := borders
	for _, cellWidth := range widths {
		tableWidth += cellWidth
	}
	if width < 70 || tableWidth > width {
		lines := []string{ansi.Truncate(title, max(width, 1), "…")}
		for _, row := range rows {
			detail := "age " + row[ageColumn]
			if showStages {
				detail += " · stages " + row[2]
			}
			if showPRs {
				detail += " · PR " + row[prColumn]
			}
			if showChecks {
				detail += " · CI " + row[ciColumn]
			}
			lines = append(lines,
				p.Title.Render(ansi.Truncate(strings.TrimSpace(row[0]+" "+row[1]), max(width, 1), "…")),
				p.Text.Render(ansi.Truncate(detail, max(width, 1), "…")), "")
		}
		return strings.TrimRight(strings.Join(lines, "\n"), "\n")
	}
	for _, row := range rows {
		for column := range row {
			row[column] = ansi.Truncate(row[column], widths[column]-2, "…")
		}
	}
	t := table.New().Headers(headers...).Rows(rows...).Width(tableWidth).Wrap(false).
		Border(lipgloss.RoundedBorder()).BorderColumn(true).BorderStyle(p.Muted).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := p.Text
			if row == table.HeaderRow {
				style = p.Title
			} else if row >= 0 && row < len(states) {
				state := states[row]
				switch {
				case state == "removed":
					style = p.Muted
				case column == 0:
					style = p.Title
				case column == prColumn:
					style = p.Title
					if workspaces[row].GitHub != nil && workspaces[row].GitHub.Error != "" {
						style = p.Warning
					}
				}
			}
			return style.Padding(0, 1).Width(widths[column]).Height(1)
		})
	return title + "\n" + t.Render()
}
