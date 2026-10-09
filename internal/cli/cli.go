package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var Version = "v1-pilot"

type Envelope struct {
	Version int      `json:"version"`
	Command string   `json:"command"`
	OK      bool     `json:"ok"`
	Data    any      `json:"data"`
	Error   *Problem `json:"error"`
}
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Session struct {
	App        *core.App
	Out        io.Writer
	Err        io.Writer
	JSON       bool
	ShowStages bool
	ShowPRs    bool
	RefreshPRs bool
	SelectPath bool
	Data       any
	Command    string
	Emitted    bool
	Ran        bool
	Root       *cobra.Command
}

func (s *Session) emit(err error) error {
	s.Emitted = true
	e := Envelope{1, s.Command, err == nil, s.Data, nil}
	if err != nil {
		e.Error = &Problem{core.ErrorCode(err), err.Error()}
	}
	if s.JSON {
		return json.NewEncoder(s.Out).Encode(e)
	}
	if err != nil {
		return s.human(s.Err, err)
	}
	return s.human(s.Out, nil)
}
func (s *Session) command(use, short, long string, args cobra.PositionalArgs, fn func(*cobra.Command, []string) (any, error)) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short, Long: long, Args: args}
	c.RunE = func(c *cobra.Command, args []string) error {
		s.Ran = true
		s.Command = c.CommandPath()
		var e error
		s.Data, e = fn(c, args)
		return e
	}
	return c
}
func (s *Session) group(use, short string) *cobra.Command {
	c := s.command(use, short, short+". Use a subcommand; --json --help returns structured help.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) { s.Data = helpData(c); return s.Data, nil })
	return c
}
func helpData(c *cobra.Command) any {
	commands := []helpEntry{}
	grouped := map[string][]helpEntry{}
	for _, child := range c.Commands() {
		if child.Hidden {
			continue
		}
		name := child.Name()
		if len(child.Aliases) > 0 {
			name += " · " + strings.Join(child.Aliases, ", ")
		}
		entry := helpEntry{name, child.Short}
		commands = append(commands, entry)
		grouped[child.GroupID] = append(grouped[child.GroupID], entry)
	}
	sections := []helpSection{}
	for _, group := range c.Groups() {
		if entries := grouped[group.ID]; len(entries) > 0 {
			sections = append(sections, helpSection{group.Title, entries})
		}
	}
	if entries := grouped[""]; len(entries) > 0 {
		sections = append(sections, helpSection{"Commands", entries})
	}
	options := []helpEntry{}
	flags := pflag.NewFlagSet("help", pflag.ContinueOnError)
	flags.AddFlagSet(c.LocalFlags())
	flags.AddFlagSet(c.InheritedFlags())
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		if f.Value.Type() != "bool" {
			name += " " + f.Value.Type()
		}
		detail := f.Usage
		if f.DefValue != "" && f.DefValue != "false" {
			detail += " (default: " + f.DefValue + ")"
		}
		options = append(options, helpEntry{name, detail})
	})
	description := c.Long
	if description == "" {
		description = c.Short
	}
	usage := c.UseLine()
	if c.Parent() == nil {
		usage = c.CommandPath()
	}
	return map[string]any{"usage": usage, "description": description, "flags": c.LocalFlags().FlagUsages() + c.InheritedFlags().FlagUsages(), "text": c.UsageString(), "commands": commands, "sections": sections, "options": options, "aliases": c.Aliases}
}
func New(app *core.App, out, errOut io.Writer) *Session {
	s := &Session{App: app, Out: out, Err: errOut}
	root := &cobra.Command{Use: "wtf", Short: "Opinionated JJ workspace management", Long: "WorkTreeForge: opinionated JJ workspace management for Omarchy.", SilenceErrors: true, SilenceUsage: true}
	root.Args = cobra.NoArgs
	root.PersistentPreRunE = func(c *cobra.Command, args []string) error { return s.applyDefaults(c) }
	s.Root = root
	root.AddGroup(
		&cobra.Group{ID: "essentials", Title: "Essentials"},
		&cobra.Group{ID: "hooks", Title: "Hooks & Tasks"},
		&cobra.Group{ID: "claims", Title: "Claims"},
		&cobra.Group{ID: "utilities", Title: "Utilities"},
	)
	root.SetHelpCommandGroupID("utilities")
	// Long-flag lookup inherits this normalization across all future subcommands.
	root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(strings.ToLower(name))
	})
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().BoolVarP(&s.JSON, "json", "j", false, "Emit one versioned JSON result; never prompt")
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		s.Command = c.CommandPath()
		s.Data = helpData(c)
		if e := s.emit(nil); e != nil {
			_, _ = fmt.Fprintln(errOut, e)
		}
	})
	root.RunE = func(c *cobra.Command, args []string) error {
		s.Command = c.CommandPath()
		s.Data = helpData(c)
		return nil
	}
	root.AddCommand(s.command("init [-- HOOK_ARGS...]", "Initialize this workspace with WTF", "Register the current JJ repository and workspace, then run its optional init hook. new uses this same registration and preparation lifecycle after creating a workspace. Repeated init preserves identity and lets reusable stages verify completed work. JSON registration codes are initialized or already_initialized; hook failures remain errors with retained stage records. Project arguments must follow --. Example: wtf init --json -- --compose.", initializationArgs, func(c *cobra.Command, args []string) (any, error) { return app.Initialize(c.Context(), args...) }))
	var from string
	n := s.command("new NAME [-- HOOK_ARGS...]", "Create a workspace and run project hooks", "Create an owned sibling JJ workspace. --from defaults to trunk(). The explicitly registered Lua override can check/block creation and delegate once to the default. Arguments after -- reach request.args unchanged; project scripts interpret them. Stages run serially; failures retain completed work. Example: wtf new agent-a -f @ -- --compose.", namedHookArgs, func(c *cobra.Command, args []string) (any, error) {
		return app.NewWorkspace(c.Context(), args[0], from, args[1:]...)
	})
	n.Flags().StringVarP(&from, "from", "f", "trunk()", "JJ parent revision for the new workspace")
	root.AddCommand(n)
	var global, here, all bool
	list := s.command("list [NAME]", "Browse and switch workspaces", "Browse active workspaces with arrow keys; Enter selects, Esc cancels. The view paginates to your terminal size. list, ls, switch, sw and w are the same command. An optional NAME resolves a workspace directly.\n\nInside a repository, show its workspaces; from a parent directory, show repositories beneath it. --global includes all registered repositories, --all includes removed records.\n\n--hook adds local stages, not CI. --pr adds GitHub PR status; --refresh forces a bounded lookup. JSON and redirected output never prompt. With shell integration, selection changes your directory.\n\nExamples:\n  wtf list\n  wtf sw -p\n  wtf ls --all --json", cobra.MaximumNArgs(1), func(c *cobra.Command, args []string) (any, error) {
		rows, e := app.List(c.Context(), global, here)
		if e != nil {
			return rows, e
		}
		if !all {
			active := make([]core.WorkspaceView, 0, len(rows))
			for _, row := range rows {
				if row.Status != "removed" {
					active = append(active, row)
				}
			}
			rows = active
		}
		if len(args) == 1 {
			for _, row := range rows {
				if row.Name == args[0] || row.ID == args[0] {
					if !row.Exists || row.Status == "removed" {
						return nil, core.Fail("workspace_unavailable", "Workspace path is unavailable")
					}
					return map[string]string{"path": row.Path, "id": row.ID}, nil
				}
			}
			return nil, core.Fail("workspace_not_found", "No workspace matches "+args[0])
		}
		if s.RefreshPRs && !s.ShowPRs {
			return nil, core.Fail("invalid_arguments", "--refresh requires --pr")
		}
		if s.ShowPRs {
			rows = s.loadPRs(c.Context(), rows)
			if c.Context().Err() != nil {
				return rows, core.Fail("command_cancelled", c.Context().Err().Error())
			}
		}
		if s.browseInteractive() {
			return s.browse(c.Context(), rows)
		}
		if s.SelectPath && !s.JSON {
			return nil, core.Fail("terminal_required", "Selection requires an interactive terminal or a workspace NAME")
		}
		return rows, nil
	})
	list.Aliases = []string{"ls", "switch", "sw", "w"}
	list.Flags().BoolVar(&global, "global", false, "List all registered repositories")
	list.Flags().BoolVar(&here, "here", false, "Only the current managed workspace")
	list.Flags().BoolVarP(&all, "all", "a", false, "Include removed workspace records; active workspaces are shown by default")
	list.Flags().BoolVar(&s.SelectPath, "select", false, "Return only the selected path; render the picker on stderr for shell navigation")
	list.Flags().BoolVarP(&s.ShowStages, "hook", "k", false, "Show local hook stages (successful/recorded), not GitHub CI")
	list.Flags().BoolVarP(&s.ShowPRs, "pr", "p", false, "Show GitHub PRs using a cache-first, bounded lookup")
	list.Flags().BoolVar(&s.RefreshPRs, "refresh", false, "Refresh PR metadata now (requires --pr)")
	list.MarkFlagsMutuallyExclusive("global", "here")
	root.AddCommand(list)
	rm := s.command("remove NAME", "Remove a workspace and its owned resources", "Invoke the target workspace's remove override before native removal. Refuses current/adopted workspaces, identity mismatch, remaining external claims and changed source files. Failures retain cleanup state. Example: wtf remove agent-a --json.", cobra.ExactArgs(1), func(c *cobra.Command, args []string) (any, error) { return app.Remove(c.Context(), args[0]) })
	rm.Aliases = []string{"rm"}
	root.AddCommand(rm)
	root.AddCommand(s.command("run TASK [ARGS...]", "Run a project task", "Run an argv-based TOML task in the current managed workspace, recording its result. Arguments after -- are forwarded literally. Tasks do not reuse results by default. Example: wtf run test -- tests/auth.", cobra.MinimumNArgs(1), func(c *cobra.Command, args []string) (any, error) { return app.Task(c.Context(), args[0], args[1:]) }))
	root.AddCommand(s.command("validate", "Check project configuration", "Parse strict config and compile the Lua file without executing it. Does not run hooks, register workspaces or snapshot JJ state. Example: wtf validate --json.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) { return app.Validate(c.Context()) }))
	doctor := s.command("doctor", "Inspect workspace health and resources", "Read current workspace identity, retained stage results, claims and failures. Does not start services or run hooks. Unhealthy state returns structured data and a nonzero exit status. Example: wtf doctor --here --json.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) { return app.Doctor(c.Context()) })
	doctor.Flags().Bool("here", true, "Inspect the current managed workspace")
	root.AddCommand(doctor)
	hooks := s.group("hook", "Inspect or invoke named hooks")
	hooks.AddCommand(s.command("list", "List configured hooks and overrides", "Load the hook module and list names. No hook function is invoked. Module declarations should be side-effect free. Example: wtf hook list --json.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) { return app.Hooks(c.Context()) }))
	hooks.AddCommand(s.command("run NAME [-- HOOK_ARGS...]", "Run a named hook", "Invoke an explicitly registered project hook under its workspace lock. Arguments after -- reach request.args unchanged. Records stages and stops on failure. Example: wtf hook run test -- --verbose.", namedHookArgs, func(c *cobra.Command, args []string) (any, error) { return app.Hook(c.Context(), args[0], args[1:]...) }))
	root.AddCommand(hooks)
	ports := s.group("ports", "Manage workspace port claims")
	ports.AddCommand(s.command("list", "List current workspace port claims", "Read claims, without checking listeners or choosing ports. Registry claims are not socket reservations. Example: wtf ports list --json.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) {
		r, w, e := current(c.Context(), app)
		if e != nil {
			return nil, e
		}
		cs, e := r.Claims(w)
		out := []core.Claim{}
		for _, claim := range cs {
			if claim.Kind == "port" {
				out = append(out, claim)
			}
		}
		return out, e
	}))
	var port int
	claim := s.command("claim NAME", "Claim a chosen loopback TCP port", "Atomically claim a port against other workspaces in this JJ repository. Project hooks choose availability/retry policy. Cross-repository leases are deferred. Example: wtf ports claim frontend --port 3000 --json.", cobra.ExactArgs(1), func(c *cobra.Command, args []string) (any, error) {
		if port < 1 || port > 65535 {
			return nil, core.Fail("invalid_port", "Port must be between 1 and 65535")
		}
		r, w, e := current(c.Context(), app)
		if e != nil {
			return nil, e
		}
		return r.Claim(c.Context(), w, "port", args[0], fmt.Sprintf("127.0.0.1:%d", port))
	})
	claim.Flags().IntVar(&port, "port", 0, "Port selected by the caller")
	_ = claim.MarkFlagRequired("port")
	ports.AddCommand(claim)
	ports.AddCommand(s.command("release NAME", "Release a current workspace port claim", "Release only the current workspace's named claim. Does not stop a service. Example: wtf ports release frontend --json.", cobra.ExactArgs(1), func(c *cobra.Command, args []string) (any, error) {
		r, w, e := current(c.Context(), app)
		if e != nil {
			return nil, e
		}
		e = r.Release(c.Context(), w, "port", args[0])
		return map[string]string{"released": args[0]}, e
	}))
	root.AddCommand(ports)
	root.AddCommand(s.command("version", "Show pilot version", "Show the local pilot version; does not update any installed binary. Example: wtf version --json.", cobra.NoArgs, func(c *cobra.Command, args []string) (any, error) { return map[string]string{"version": Version}, nil }))
	root.CompletionOptions.DisableDefaultCmd = true
	for _, command := range root.Commands() {
		switch command.Name() {
		case "init", "list", "new", "remove":
			command.GroupID = "essentials"
		case "hook", "run":
			command.GroupID = "hooks"
		case "ports":
			command.GroupID = "claims"
		default:
			command.GroupID = "utilities"
		}
	}
	return s
}
func current(ctx context.Context, a *core.App) (core.Repo, core.Workspace, error) {
	r, e := core.Discover(ctx, a.Cwd)
	if e != nil {
		return r, core.Workspace{}, e
	}
	w, e := r.Workspace(ctx)
	return r, w, e
}
func (s *Session) Execute(ctx context.Context, args []string) int {
	input := prepareInput(s.Root, args)
	// This survives parse errors and does not mistake child argv or flag values for JSON.
	s.JSON = input.JSON
	s.Root.SetArgs(input.Args)
	c, e := s.Root.ExecuteContextC(ctx)
	if s.Command == "" {
		s.Command = "wtf"
		if c != nil {
			s.Command = c.CommandPath()
		}
	}
	if e != nil && !s.Ran && core.ErrorCode(e) == "operation_failed" {
		e = core.Fail("invalid_arguments", e.Error())
	}
	if !s.Emitted {
		if writeErr := s.emit(e); writeErr != nil {
			_, _ = fmt.Fprintln(s.Err, writeErr)
			return 1
		}
	}
	if e != nil {
		return 1
	}
	return 0
}
func Execute(ctx context.Context, args []string) int {
	cwd, e := os.Getwd()
	if e != nil {
		_, _ = fmt.Fprintln(os.Stderr, e)
		return 1
	}
	home, e := core.Home()
	if e != nil {
		_, _ = fmt.Fprintln(os.Stderr, e)
		return 1
	}
	return New(core.New(cwd, home, os.Stderr), os.Stdout, os.Stderr).Execute(ctx, args)
}
