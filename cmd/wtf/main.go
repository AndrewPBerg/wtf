package main

import (
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, cli.FormatError(err))
		os.Exit(1)
	}
}
