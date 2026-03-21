package cli

import "github.com/fatih/color"

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
