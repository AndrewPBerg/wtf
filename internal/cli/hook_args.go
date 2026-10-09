package cli

import (
	"fmt"
	"github.com/spf13/cobra"
)

// Project arguments live after --, without becoming Go flags or losing case.
func namedHookArgs(command *cobra.Command, args []string) error {
	if err := cobra.MinimumNArgs(1)(command, args); err != nil {
		return err
	}
	if boundary := command.ArgsLenAtDash(); boundary >= 0 {
		if boundary != 1 {
			return fmt.Errorf("put the name before -- and hook arguments after it")
		}
	} else if len(args) != 1 {
		return fmt.Errorf("hook arguments must follow --")
	}
	return nil
}

func initializationArgs(command *cobra.Command, args []string) error {
	if len(args) > 0 && command.ArgsLenAtDash() != 0 {
		return fmt.Errorf("init hook arguments must follow --")
	}
	return nil
}
