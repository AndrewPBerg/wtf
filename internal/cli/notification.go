package cli

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/muesli/termenv"
)

func (s *Session) notify(note core.Notification) error {
	p := humanPresentation(s.Err)
	if s.JSON || !terminalWriter(s.Out) || !terminalWriter(s.Err) {
		// Even FORCE_COLOR cannot decorate JSON-mode or redirected hook logs.
		p.Renderer.SetColorProfile(termenv.Ascii)
	}
	_, err := fmt.Fprintln(s.Err, notificationText(p, note))
	return err
}

func notificationText(p presentation, note core.Notification) string {
	message := plain(note.Message)
	switch note.Level {
	case "success":
		return p.Good.Render("✓ " + message)
	case "warning":
		return p.Warning.Render("! " + message)
	case "error":
		return p.Bad.Render("✗ " + message)
	default:
		return p.Title.Render("•") + " " + p.Text.Render(message)
	}
}
