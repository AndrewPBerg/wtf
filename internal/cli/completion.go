package cli

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type completionScript struct {
	Shell  string `json:"shell"`
	Script string `json:"script"`
}

func (s *Session) addCompletion() {
	command := s.command("completion SHELL", "Generate Bash, Zsh or Fish shell completions", "Print a shell script for bash, zsh or fish. Source it in your shell to complete commands, aliases, flags, live workspace names and configured project tasks. Workspace and task matches include fuzzy subsequences. No hooks or network lookups run. Example: source <(wtf completion bash). See docs/completion.md for installation and shell aliases.", cobra.ExactArgs(1), func(c *cobra.Command, args []string) (any, error) {
		shell := strings.ToLower(args[0])
		script, ok := completionScripts[shell]
		if !ok {
			return nil, core.Fail("invalid_arguments", "Expected bash, zsh or fish")
		}
		return completionScript{shell, script}, nil
	})
	command.Annotations = map[string]string{"wtf.no-hooks": "true"}
	s.Root.AddCommand(command)
}

// complete is the shell-only protocol; it never executes a command or applies defaults.
func (s *Session) complete(ctx context.Context, words []string) int {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if len(words) == 0 {
		words = []string{""}
	}
	partial := words[len(words)-1]
	words = prepareInput(s.Root, words[:len(words)-1]).Args
	command := s.Root
	args := []string{}
	var pending *pflag.Flag
	flags := map[string]string{}
	separated := false
	for _, word := range words {
		if pending != nil {
			flags[pending.Name] = word
			pending = nil
			continue
		}
		if word == "--" {
			separated = true
			continue
		}
		if separated {
			args = append(args, word)
			continue
		}
		if strings.HasPrefix(word, "-") && word != "-" {
			name, value, attached := strings.Cut(strings.TrimLeft(word, "-"), "=")
			short := !strings.HasPrefix(word, "--")
			if short {
				for i := 0; i < len(name); i++ {
					flag := findFlag(command, strings.ToLower(name[i:i+1]), true)
					if flag == nil {
						continue
					}
					if flag.NoOptDefVal == "" {
						if i+1 < len(name) {
							flags[flag.Name] = name[i+1:]
						} else if attached {
							flags[flag.Name] = value
						} else {
							pending = flag
						}
						break
					}
					flags[flag.Name] = flag.NoOptDefVal
					if attached {
						flags[flag.Name] = value
					}
				}
			} else if flag := findFlag(command, strings.ToLower(name), false); flag != nil {
				if attached {
					flags[flag.Name] = value
				} else if flag.NoOptDefVal == "" {
					pending = flag
				} else {
					flags[flag.Name] = flag.NoOptDefVal
				}
			}
			continue
		}
		matched := false
		if len(args) == 0 {
			for _, child := range command.Commands() {
				if commandMatches(child, word) {
					command = child
					matched = true
					break
				}
			}
		}
		if !matched {
			args = append(args, word)
		}
	}
	candidates := map[string]string{}
	add := func(name, description string, fuzzy bool) {
		if completionMatch(name, partial, fuzzy) {
			candidates[name] = plain(strings.ReplaceAll(strings.ReplaceAll(description, "\n", " "), "\t", " "))
		}
	}
	directive := cobra.ShellCompDirectiveNoFileComp
	if separated {
		directive = cobra.ShellCompDirectiveDefault
	} else if pending != nil {
		if pending.Value.Type() == "bool" {
			add("true", "", false)
			add("false", "", false)
		} else if pending.Name != "from" && pending.Name != "message" {
			directive = cobra.ShellCompDirectiveDefault
		}
	} else if strings.HasPrefix(partial, "-") {
		name, _, attached := strings.Cut(partial, "=")
		if attached {
			flag := findFlag(command, strings.ToLower(strings.TrimLeft(name, "-")), !strings.HasPrefix(name, "--"))
			if flag != nil && flag.Value.Type() == "bool" {
				for _, value := range []string{"true", "false"} {
					add(name+"="+value, "", false)
				}
			}
		} else {
			visit := func(flag *pflag.Flag) {
				if !flag.Hidden {
					add("--"+flag.Name, flag.Usage, false)
					if flag.Shorthand != "" {
						add("-"+flag.Shorthand, flag.Usage, false)
					}
				}
			}
			command.InitDefaultHelpFlag()
			command.Flags().VisitAll(visit)
			for parent := command; parent != nil; parent = parent.Parent() {
				parent.PersistentFlags().VisitAll(visit)
			}
		}
	} else if len(args) == 0 {
		for _, child := range command.Commands() {
			if child.Hidden {
				continue
			}
			add(child.Name(), child.Short, false)
			for _, alias := range child.Aliases {
				add(alias, child.Short, false)
			}
		}
		switch command.CommandPath() {
		case "wtf list", "wtf remove", "wtf change":
			rows, _ := s.App.CompletionWorkspaces(ctx, strings.EqualFold(flags["global"], "true"), strings.EqualFold(flags["here"], "true"))
			for _, row := range rows {
				if row.Exists && row.Status != "removed" {
					add(row.Name, row.Message, true)
				}
			}
		case "wtf run":
			// Discover resolves subdirectories to the same workspace config used by run.
			if repo, err := core.Discover(ctx, s.App.Cwd); err == nil {
				if config, err := core.LoadConfig(repo.Root); err == nil {
					for name, task := range config.Tasks {
						add(name, strings.Join(task.Command, " "), true)
					}
				}
			}
		case "wtf completion":
			for _, shell := range []string{"bash", "zsh", "fish"} {
				add(shell, "", false)
			}
		case "wtf new":
			directive = cobra.ShellCompDirectiveDefault
		}
	} else if command.CommandPath() == "wtf run" {
		directive = cobra.ShellCompDirectiveDefault
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := strings.HasPrefix(strings.ToLower(names[i]), strings.ToLower(partial))
		right := strings.HasPrefix(strings.ToLower(names[j]), strings.ToLower(partial))
		if left != right {
			return left
		}
		return names[i] < names[j]
	})
	var output bytes.Buffer
	for _, name := range names {
		fmt.Fprintln(&output, name+"\t"+candidates[name])
	}
	fmt.Fprintf(&output, ":%d\n", directive)
	if _, err := s.Out.Write(output.Bytes()); err != nil {
		return 1
	}
	return 0
}

func completionMatch(name, query string, fuzzy bool) bool {
	name, query = strings.ToLower(name), strings.ToLower(query)
	if strings.HasPrefix(name, query) {
		return true
	}
	if !fuzzy {
		return false
	}
	for _, character := range query {
		index := strings.IndexRune(name, character)
		if index < 0 {
			return false
		}
		name = name[index+len(string(character)):]
	}
	return true
}
