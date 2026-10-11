package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// titledPanel puts the caption in the upper-left edge of a rounded frame.
func titledPanel(p presentation, title, content string, width int) string {
	if width < 12 {
		return p.Title.Render(ansi.Truncate(plain(title), max(1, width), "…")) + "\n" + content
	}
	body := p.Renderer.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true).
		BorderForeground(p.Muted.GetForeground()).Padding(0, 1).Width(width - 2).Render(content)
	frameWidth := lipgloss.Width(body)
	title = ansi.Truncate(plain(title), max(1, frameWidth-6), "…")
	top := p.Muted.Render("╭─ ") + p.Title.Render(title) +
		p.Muted.Render(" "+strings.Repeat("─", max(0, frameWidth-lipgloss.Width(title)-5))+"╮")
	return top + "\n" + body
}
