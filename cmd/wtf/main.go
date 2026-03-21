package main

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/cli"
	"github.com/fatih/color"
)

func main() {
	if err := cli.Execute(); err != nil {
		red := color.New(color.FgRed, color.Bold).SprintFunc()
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
}
