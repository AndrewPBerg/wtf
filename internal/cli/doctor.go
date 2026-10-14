package cli

import (
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func doctorHealthText(p presentation, health core.Health) string {
	lines := []string{}
	if health.Workspace.Name != "" && len(health.Issues) == 0 {
		lines = append(lines, p.Good.Render("✓ Workspace healthy")+"  "+p.Title.Render(plain(health.Workspace.Name)), p.Muted.Render(fmt.Sprintf("%d stages · %d resource claims", len(health.Workspace.Stages), len(health.Claims))))
	}
	for _, item := range []struct {
		name  string
		check core.DiagnosticCheck
	}{{"Configuration", health.Configuration}, {"Workspace", health.WorkspaceCheck}} {
		if item.check.Status == "" {
			continue
		}
		style, icon := p.Good, "✓"
		if item.check.Status != "ready" {
			style, icon = p.Warning, "!"
		}
		if item.check.Status == "not_configured" {
			style, icon = p.Muted, "–"
		}
		if item.check.Status == "failed" {
			style, icon = p.Bad, "×"
		}
		lines = append(lines, style.Render(icon)+" "+p.Title.Render(item.name)+"  "+style.Render(plain(item.check.Status)), "  "+p.Muted.Render(plain(item.check.Message)))
		if item.check.Remedy != "" {
			lines = append(lines, "  "+p.Title.Render("→")+" "+p.Muted.Render(plain(item.check.Remedy)))
		}
	}
	lines = append(lines, doctorToolsText(p, health.Tools))
	return strings.Join(lines, "\n")
}

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
