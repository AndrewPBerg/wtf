package cli

import (
	"context"
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

func (s *Session) teardownSetup(ctx context.Context) (core.SetupTeardownResult, error) {
	result, err := s.App.TeardownSetup(ctx, false)
	if err != nil || result.Status == "not_configured" {
		return result, err
	}
	if !s.browseInteractive() {
		return result, core.Fail("confirmation_required", "Run wtf setup teardown in an interactive terminal to confirm removal of "+result.Path)
	}
	model, err := tea.NewProgram(setupTeardownPrompt{path: result.Path, p: humanPresentation(s.Out)}, tea.WithInput(s.Root.InOrStdin()), tea.WithOutput(s.Out), tea.WithContext(ctx)).Run()
	if err != nil {
		return result, err
	}
	decision, ok := model.(setupTeardownPrompt)
	if !ok {
		return result, core.Fail("confirmation_failed", "Teardown confirmation returned an invalid result")
	}
	if !decision.confirmed {
		result.Status = "cancelled"
		return result, nil
	}
	return s.App.TeardownSetup(ctx, true)
}

type setupTeardownPrompt struct {
	path      string
	p         presentation
	yes       bool
	confirmed bool
}

func (m setupTeardownPrompt) Init() tea.Cmd { return nil }

func (m setupTeardownPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "left", "right", "tab", "shift+tab":
			m.yes = !m.yes
		case "y", "Y":
			m.yes = true
		case "n", "N":
			m.yes = false
		case "enter":
			m.confirmed = m.yes
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupTeardownPrompt) View() string {
	yes, no := m.p.Muted.Render("Yes"), m.p.Title.Render("[No]")
	if m.yes {
		yes, no = m.p.Bad.Bold(true).Render("[Yes]"), m.p.Muted.Render("No")
	}
	return fmt.Sprintf("Remove %s and all its contents?\n\n  %s   %s\n\n%s\n", m.p.Text.Render(plain(m.path)), yes, no, m.p.Muted.Render("←/→ or y/n to select · Enter to confirm · Esc to cancel"))
}
