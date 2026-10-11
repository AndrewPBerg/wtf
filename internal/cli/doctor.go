package cli

import (
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func doctorToolsText(p presentation, checks []core.ToolCheck) string {
	lines := []string{p.Title.Render("Tools")}
	for _, check := range checks {
		requirement := "optional"
		icon, statusStyle := "✓", p.Good
		if check.Status != "ready" {
			icon, statusStyle = "!", p.Warning
			if check.Required {
				icon, statusStyle = "×", p.Bad
			}
		}
		if check.Required {
			requirement = "required"
		}
		lines = append(lines,
			"  "+statusStyle.Render(icon)+" "+p.Title.Render(plain(check.Tool))+"  "+statusStyle.Render(plain(check.Status))+"  "+p.Muted.Render(requirement+" · "+plain(check.Feature)),
		)
		if check.Tool == "linear" && check.Message != "" {
			lines = append(lines, "    "+p.Muted.Render(plain(check.Message)))
		}
		if check.Remedy != "" {
			lines = append(lines, "    "+p.Title.Render("→")+" "+p.Muted.Render(plain(check.Remedy)))
		}
	}
	return strings.Join(lines, "\n")
}
