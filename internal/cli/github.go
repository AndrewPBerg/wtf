package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

func (s *Session) loadPRs(ctx context.Context, rows []core.WorkspaceView) []core.WorkspaceView {
	plan := s.App.Metadata().Prepare(ctx, rows, core.MetadataRequest{
		PRs: s.ShowPRs, CI: s.ShowChecks, Reviews: s.ShowPRs, ForceReviews: s.RefreshPRs,
	})
	interactive := false
	if file, ok := s.Out.(interface{ Fd() uintptr }); ok {
		interactive = !s.JSON && !s.browseInteractive() && term.IsTerminal(file.Fd())
	}
	if interactive {
		// Cache-first terminal output; JSON still emits exactly once, after bounded refresh.
		p := humanPresentation(s.Out)
		preview := workspaceTable(p, plan.Cached, s.App.Cwd, s.ShowStages, s.ShowPRs, s.ShowChecks)
		_, _ = fmt.Fprintln(s.Out, preview)
		refreshed := plan.Refresh(ctx)
		lines := lipgloss.Height(preview)
		// Move only over the preview, never clear the user's scrollback.
		_, _ = fmt.Fprintf(s.Out, "\x1b[%dA\r\x1b[J", lines)
		return refreshed
	}
	return plan.Refresh(ctx)
}

// Only terminal output emits OSC 8 links; JSON retains GitHub's URL as data.
func prPresentation(p presentation, info *core.PRInfo) string {
	label := prLabel(info)
	pr, matched := primaryPR(info)
	if !p.Links || !matched {
		return label
	}
	target := pr.URL
	if pr.Review != nil && pr.Review.Provider == "linear" {
		target = pr.Review.URL
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "github.com" && parsed.Host != "linear.app") || parsed.User != nil || plain(target) != target {
		return label
	}
	glyph, style := "\uf407", p.Warning
	switch strings.ToUpper(pr.State) {
	case "OPEN":
		style = p.Good
		if pr.Draft {
			glyph, style = "\uf4dd", p.Muted
		}
	case "CLOSED":
		glyph, style = "\uf4dc", p.Bad
	case "MERGED":
		glyph, style = "\uf419", p.Title
	}
	return ansi.SetHyperlink(target) + style.Render(glyph+" "+label) + ansi.ResetHyperlink()
}

func primaryPR(info *core.PRInfo) (core.PullRequest, bool) {
	if info != nil {
		for _, pr := range info.Matches {
			return pr, true
		}
	}
	return core.PullRequest{}, false
}

func ciLabel(info *core.PRInfo) string {
	if info != nil && info.CIState == core.MetadataNone {
		return ""
	}
	pr, matched := primaryPR(info)
	if !matched {
		if info == nil {
			return "?"
		}
		if info.Error != "" {
			return "?"
		}
		switch info.Lookup {
		case "fresh", "cached", "not_github", "not_inspected":
			if info.Lookup == "fresh" || info.Lookup == "cached" {
				return ""
			}
			return "—"
		case "loading":
			return "↻"
		default:
			return "?"
		}
	}
	c := pr.Checks
	if c == nil || c.Head != pr.Head {
		return "?"
	}
	parts := []string{}
	if c.Failed > 0 {
		parts = append(parts, fmt.Sprintf("✗ %d", c.Failed))
	}
	if c.Cancelled > 0 {
		parts = append(parts, fmt.Sprintf("⊘ %d", c.Cancelled))
	}
	if c.Pending > 0 {
		parts = append(parts, fmt.Sprintf("↻ %d", c.Pending))
	}
	if c.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("? %d", c.Unknown))
	} else if c.Truncated {
		parts = append(parts, "?")
	}
	if len(parts) == 0 && c.Passed > 0 {
		parts = append(parts, fmt.Sprintf("✓ %d", c.Passed))
	} else if len(parts) == 0 && c.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("⊘ %d", c.Skipped))
	}
	label := strings.Join(parts, " ")
	if label == "" && (info.Lookup == "stale" || info.Error != "") {
		return "?"
	}
	if label != "" && (info.Lookup == "stale" || info.Error != "") {
		label += " ~"
	}
	return label
}

func ciPresentation(p presentation, info *core.PRInfo) string {
	label := ciLabel(info)
	pr, matched := primaryPR(info)
	style := p.Warning
	if matched && pr.Checks != nil && pr.Checks.Head == pr.Head {
		if pr.Checks.Failed > 0 {
			style = p.Bad
		} else if strings.HasPrefix(label, "✓") && info.Lookup != "stale" && info.Error == "" {
			style = p.Good
		}
	}
	text := style.Render(label)
	if p.Links && label != "" && matched {
		parsed, err := url.Parse(pr.URL)
		if err == nil && parsed.Scheme == "https" && parsed.Host == "github.com" && parsed.User == nil && plain(pr.URL) == pr.URL {
			return ansi.SetHyperlink(strings.TrimRight(pr.URL, "/")+"/checks") + text + ansi.ResetHyperlink()
		}
	}
	return text
}

func prLabel(info *core.PRInfo) string {
	if info == nil {
		return "unknown"
	}
	if info.PRState == core.MetadataNone {
		return ""
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
		return ""
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
