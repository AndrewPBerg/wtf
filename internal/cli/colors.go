package cli

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	green = color.New(color.FgGreen).SprintFunc()
	cyan  = color.New(color.FgCyan).SprintFunc()

	yellow = color.New(color.FgYellow).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
	dim    = color.New(color.FgHiBlack).SprintFunc()

	greenBold = color.New(color.FgGreen, color.Bold).SprintFunc()
	cyanBold  = color.New(color.FgCyan, color.Bold).SprintFunc()
	redBold   = color.New(color.FgRed, color.Bold).SprintFunc()
)

// hyperlink wraps text in an OSC 8 terminal hyperlink escape sequence.
// Terminals that support OSC 8 (most modern ones) will make the text
// ctrl+clickable, opening the URL in the default browser.
func hyperlink(url, text string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}
