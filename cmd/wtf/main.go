package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AndrewPBerg/wtf/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if cli.IsJSONOutput() {
			_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, cli.FormatError(err))
		}
		os.Exit(1)
	}
}
