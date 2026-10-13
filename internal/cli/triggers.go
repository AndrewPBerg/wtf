package cli

import (
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func (s *Session) invokeHooks(c *cobra.Command, args []string, operation func(*cobra.Command, []string) (any, error)) (any, error) {
	command := strings.TrimPrefix(c.CommandPath(), "wtf ")
	if command == "change" && !c.Flags().Changed("message") {
		return operation(c, args) // No requested mutation: help must not run callbacks.
	}
	if c.Annotations["wtf.no-hooks"] == "true" || command == "version" || command == "doctor" || command == "hook list" || command == "setup teardown" {
		return operation(c, args)
	}
	if command == "remove" && len(args) == 0 && !s.browseInteractive() {
		// Redirected/JSON removal without a target is an inspection preview.
		return operation(c, args)
	}
	invocation := core.HookInvocation{Command: command, Args: args}
	if command == "new" && len(args) > 0 {
		path, err := s.App.ResolveWorkspacePath(c.Context(), args[0])
		if err != nil {
			return operation(c, args)
		}
		invocation.Path = path
		invocation.From, _ = c.Flags().GetString("from")
	}
	return s.App.InvokeHooks(c.Context(), invocation, func() (any, error) { return operation(c, args) })
}
