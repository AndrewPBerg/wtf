package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandInput struct {
	Args []string
	JSON bool
}

// prepareInput is command-tree middleware: no command maintains its own case rules.
// Only CLI syntax and boolean literals are folded; opaque values and child argv survive.
func prepareInput(root *cobra.Command, args []string) commandInput {
	root.InitDefaultHelpCmd()
	input := commandInput{Args: append([]string{}, args...)}
	result := input.Args
	observe := func(flag *pflag.Flag, value string) {
		if flag != nil && flag.Name == "json" {
			if enabled, err := strconv.ParseBool(value); err == nil {
				input.JSON = enabled
			}
		}
	}
	command := root
	commandPosition, value := true, false
	for i, arg := range result {
		if value {
			value = false
			continue
		}
		if arg == "--" {
			break
		}
		command.InitDefaultHelpFlag()
		if strings.HasPrefix(arg, "--") {
			name, attached, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			name = strings.ToLower(name)
			flag := findFlag(command, name, false)
			result[i] = "--" + name
			if hasValue {
				attached = flagValue(flag, attached)
				result[i] += "=" + attached
				observe(flag, attached)
			} else if flag != nil {
				value = flag.NoOptDefVal == ""
				observe(flag, flag.NoOptDefVal)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			short := strings.TrimPrefix(arg, "-")
			folded := "-"
			for offset := 0; offset < len(short); offset++ {
				name := strings.ToLower(short[offset : offset+1])
				flag := findFlag(command, name, true)
				if flag != nil {
					folded += flag.Shorthand
				} else {
					folded += name
				}
				if flag == nil || flag.NoOptDefVal == "" {
					// A non-boolean shorthand consumes the remaining bytes as its value.
					folded += short[offset+1:]
					value = flag != nil && offset+1 == len(short)
					break
				}
				if offset+1 < len(short) && short[offset+1] == '=' {
					attached := flagValue(flag, short[offset+2:])
					folded += "=" + attached
					observe(flag, attached)
					break
				}
				observe(flag, flag.NoOptDefVal)
			}
			result[i] = folded
			continue
		}
		if commandPosition {
			matched := false
			for _, child := range command.Commands() {
				if commandMatches(child, arg) {
					result[i] = child.Name()
					command = child
					matched = true
					break
				}
			}
			if !matched {
				commandPosition = false
			}
		}
	}
	return input
}

func flagValue(flag *pflag.Flag, value string) string {
	if flag != nil && flag.Value.Type() == "bool" {
		return strings.ToLower(value)
	}
	return value
}

func commandMatches(command *cobra.Command, name string) bool {
	if strings.EqualFold(command.Name(), name) {
		return true
	}
	for _, alias := range command.Aliases {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

func findFlag(command *cobra.Command, name string, shorthand bool) *pflag.Flag {
	lookup := func(flags *pflag.FlagSet) *pflag.Flag {
		if shorthand {
			var match *pflag.Flag
			flags.VisitAll(func(flag *pflag.Flag) {
				if strings.EqualFold(flag.Shorthand, name) {
					match = flag
				}
			})
			return match
		}
		return flags.Lookup(name)
	}
	if flag := lookup(command.Flags()); flag != nil {
		return flag
	}
	for parent := command; parent != nil; parent = parent.Parent() {
		if flag := lookup(parent.PersistentFlags()); flag != nil {
			return flag
		}
	}
	return nil
}
