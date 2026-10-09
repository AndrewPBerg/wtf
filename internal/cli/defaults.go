package cli

import (
	"strconv"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func (s *Session) applyDefaults(command *cobra.Command) error {
	defaults, e := s.App.CommandDefaults(command.Context(), func(key string) (string, error) {
		target := s.Root
		for _, part := range strings.Fields(key) {
			matched := false
			for _, child := range target.Commands() {
				if commandMatches(child, part) {
					target = child
					matched = true
					break
				}
			}
			if !matched {
				return "", core.Fail("invalid_config", "Unknown configured command: "+key)
			}
		}
		return target.CommandPath(), nil
	})
	if e != nil {
		return e
	}
	merged := defaults[command.CommandPath()]
	for name, value := range merged {
		flag := findFlag(command, name, false)
		if flag == nil {
			return core.Fail("invalid_config", "Unknown default flag for "+command.CommandPath()+": "+name)
		}
		if flag.Changed {
			continue
		}
		var text string
		switch v := value.(type) {
		case bool:
			text = strconv.FormatBool(v)
		case int64:
			text = strconv.FormatInt(v, 10)
		case string:
			text = v
		default:
			return core.Fail("invalid_config", "Unsupported default flag type")
		}
		// Let the registered flag type validate its value. New flag types inherit
		// this path without adding command-specific conversion rules.
		if e = flag.Value.Set(flagValue(flag, text)); e != nil {
			return core.Fail("invalid_config", "Default "+name+": "+e.Error())
		}
		flag.Changed = true
	}
	return nil
}
