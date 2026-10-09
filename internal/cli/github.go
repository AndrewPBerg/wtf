package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

func (s *Session) loadPRs(ctx context.Context, rows []core.WorkspaceView) []core.WorkspaceView {
	service := s.App.GitHubService()
	plan := service.Prepare(ctx, rows, s.RefreshPRs)
	if !plan.NeedsRefresh {
		return plan.Rows
	}
	interactive := false
	if file, ok := s.Out.(interface{ Fd() uintptr }); ok {
		interactive = !s.JSON && !s.browseInteractive() && term.IsTerminal(file.Fd())
	}
	if interactive {
		// Cache-first terminal output; JSON still emits exactly once, after bounded refresh.
		p := humanPresentation(s.Out)
		preview := workspaceTable(p, plan.Rows, s.App.Cwd, s.ShowStages, true)
		_, _ = fmt.Fprintln(s.Out, preview)
		refreshed := service.Refresh(ctx, plan)
		lines := lipgloss.Height(preview)
		// Move only over the preview, never clear the user's scrollback.
		_, _ = fmt.Fprintf(s.Out, "\x1b[%dA\r\x1b[J", lines)
		return refreshed
	}
	return service.Refresh(ctx, plan)
}

func prLabel(info *core.PRInfo) string {
	if info == nil {
		return "unknown"
	}
	if len(info.Matches) > 0 {
		pr := info.Matches[0]
		state := strings.ToLower(pr.State)
		if pr.Draft && pr.State == "OPEN" {
			state = "draft"
		}
		label := fmt.Sprintf("#%d %s", pr.Number, state)
		if len(info.Matches) > 1 {
			label += fmt.Sprintf(" +%d", len(info.Matches)-1)
		}
		if info.Lookup == "stale" || info.Error != "" {
			label += " ~"
		}
		return label
	}
	switch info.Lookup {
	case "fresh", "cached":
		return "unassociated"
	case "stale":
		return "stale"
	case "not_github", "not_inspected":
		return "—"
	case "loading":
		return "loading…"
	default:
		return "unavailable"
	}
}
