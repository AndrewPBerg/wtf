package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Project arguments live after --, without becoming Go flags or losing case.
func namedHookArgs(command *cobra.Command, args []string) error {
	return targetHookArgs(command, args, "name")
}

func pathHookArgs(command *cobra.Command, args []string) error {
	return targetHookArgs(command, args, "path")
}

func targetHookArgs(command *cobra.Command, args []string, target string) error {
	if err := cobra.MinimumNArgs(1)(command, args); err != nil {
		return err
	}
	if boundary := command.ArgsLenAtDash(); boundary >= 0 {
		if boundary != 1 {
			return fmt.Errorf("put the %s before -- and hook arguments after it", target)
		}
	} else if len(args) != 1 {
		return fmt.Errorf("hook arguments must follow --")
	}
	return nil
}
