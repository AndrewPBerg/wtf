// Package terminal configures shared terminal defaults before the UI starts.
package terminal

import "github.com/charmbracelet/lipgloss"

func init() {
	// WTF uses explicit Omarchy colors rather than adaptive background colors.
	// Bubble Tea v1 queries the default Lip Gloss background during package init,
	// which otherwise writes an OSC query to stdout even for --json. Go initializes
	// ready packages in import-path order: this package precedes Bubble Tea after
	// their shared Lip Gloss dependency. Keep the real-PTY JSON check when changing
	// the module path or UI dependency; setting this in main would be too late.
	lipgloss.SetHasDarkBackground(true)
}
