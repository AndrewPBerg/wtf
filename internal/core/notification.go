package core

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Notification is a host-rendered script message, independent of terminal styles.
type Notification struct {
	Message string
	Level   string
}

// Notify routes a script message through the host UI, or plain logs for embedders.
// Severity is presentation only: an error notification does not fail the hook.
func (a *App) Notify(message, level string) error {
	if level == "" {
		level = "info"
	}
	level = strings.ToLower(level)
	switch level {
	case "info", "success", "warning", "error":
	default:
		return Fail("invalid_notification", "Notification level must be info, success, warning or error")
	}
	message = strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, ansi.Strip(message))
	note := Notification{Message: message, Level: level}
	if a.Notifier != nil {
		return a.Notifier(note)
	}
	_, err := fmt.Fprintln(a.Out, "["+note.Level+"] "+note.Message)
	return err
}
