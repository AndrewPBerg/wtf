package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type helpEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type helpSection struct {
	Title   string      `json:"title"`
	Entries []helpEntry `json:"entries"`
}

func helpPresentation(p presentation, data map[string]any) string {
	width := max(1, min(p.Width-1, 110))
	inner := max(1, width-4)
	wrap := func(value string, cells int) string {
		lines := []string{}
		for _, paragraph := range strings.Split(value, "\n") {
			lines = append(lines, ansi.Hardwrap(ansi.Wrap(plain(paragraph), cells, ""), cells, true))
		}
		return strings.Join(lines, "\n")
	}
	usage, _ := data["usage"].(string)
	description, _ := data["description"].(string)
	intro := p.Text.Render(wrap(description, inner))
	if aliases, ok := data["aliases"].([]string); ok && len(aliases) > 0 {
		intro += "\n\n" + p.Muted.Render(wrap("Aliases: "+strings.Join(aliases, ", "), inner))
	}
	panel := func(title, content string) string {
		if width < 12 {
			return p.Title.Render(wrap(title, inner)) + "\n" + content
		}
		body := p.Renderer.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true).BorderForeground(p.Muted.GetForeground()).Padding(0, 1).Width(inner + 2).Render(content)
		frameWidth := lipgloss.Width(body)
		title = ansi.Truncate(plain(title), max(1, frameWidth-6), "…")
		top := p.Muted.Render("╭─ ") + p.Title.Render(title) + p.Muted.Render(" "+strings.Repeat("─", max(0, frameWidth-lipgloss.Width(title)-5))+"╮")
		return top + "\n" + body
	}
	sections := []string{panel(usage, intro)}
	groups, _ := data["sections"].([]helpSection)
	if len(groups) == 0 {
		commands, _ := data["commands"].([]helpEntry)
		groups = []helpSection{{"Commands", commands}}
	}
	options, _ := data["options"].([]helpEntry)
	groups = append(append([]helpSection{}, groups...), helpSection{"Flags", options})
	for _, section := range groups {
		entries := section.Entries
		if len(entries) == 0 {
			continue
		}
		nameWidth := 0
		for _, entry := range entries {
			nameWidth = max(nameWidth, lipgloss.Width(entry.Name))
		}
		nameWidth = min(nameWidth, inner/2)
		lines := []string{}
		for _, entry := range entries {
			if inner < 55 {
				lines = append(lines, p.Title.Render(wrap(entry.Name, inner)), p.Text.Render(wrap(entry.Description, inner)), "")
				continue
			}
			names := strings.Split(wrap(entry.Name, nameWidth), "\n")
			details := strings.Split(wrap(entry.Description, inner-nameWidth-3), "\n")
			for line := 0; line < max(len(names), len(details)); line++ {
				name, detail := "", ""
				for i, value := range names {
					if i == line {
						name = value
					}
				}
				for i, value := range details {
					if i == line {
						detail = value
					}
				}
				lines = append(lines, p.Title.Width(nameWidth).Render(name)+"   "+p.Text.Render(detail))
			}
		}
		sections = append(sections, panel(section.Title, strings.TrimRight(strings.Join(lines, "\n"), "\n")))
	}
	return strings.Join(sections, "\n")
}
