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

// Version identifies the installed WTF build.
var Version = "1.0.0"

// Envelope is the versioned JSON command result.
type Envelope struct {
	Version int      `json:"version"`
	Command string   `json:"command"`
	OK      bool     `json:"ok"`
	Data    any      `json:"data"`
	Error   *Problem `json:"error"`
}

// Problem is a stable command error code and message.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Session binds command flags, output and the workspace application.
type Session struct {
	App           *core.App
	Out           io.Writer
	Err           io.Writer
	JSON          bool
	ShowStages    bool
	ShowPRs       bool
	ShowChecks    bool
	RefreshPRs    bool
	SelectPath    bool
	Data          any
	Command       string
	Emitted       bool
	Ran           bool
	Root          *cobra.Command
	DefaultsError error
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
		s.Data, e = s.invokeHooks(c, args, fn)
		if e != nil {
			// A cancelled picker suppresses its success result. A subsequent
			// hook failure still needs an error report.
			s.Emitted = false
		}
		return e
	}
	return c
}
func (s *Session) group(use, short string) *cobra.Command {
	c := s.command(use, short, short+". Use a subcommand; --json --help returns structured help.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) { s.Data = helpData(c); return s.Data, nil })
	c.Annotations = map[string]string{"wtf.no-hooks": "true"}
	return c
}
func helpData(c *cobra.Command) any {
	commands := []helpEntry{}
	grouped := map[string][]helpEntry{}
	for _, child := range c.Commands() {
		if child.Hidden || child.Name() == "help" {
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

// New constructs the WTF command tree.
func New(app *core.App, out, errOut io.Writer) *Session {
	s := &Session{App: app, Out: out, Err: errOut}
	app.Notifier = s.notify
	root := &cobra.Command{Use: "wtf", Short: "Opinionated JJ workspace management", Long: "WorkTreeForge: opinionated JJ workspace management for Omarchy, Zed, and Linear Workflows.", SilenceErrors: true, SilenceUsage: true}
	root.Args = cobra.NoArgs
	var showVersion bool
	root.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		if showVersion || c.Name() == "version" || c.Name() == "completion" {
			return nil
		}
		err := s.applyDefaults(c)
		if c.CommandPath() == "wtf setup teardown" {
			// Local cleanup remains available even when its config is broken.
			return nil
		}
		if c.Name() == "doctor" {
			// Diagnostics must remain usable when configuration is broken.
			s.DefaultsError = err
			return nil
		}
		return err
	}
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
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "Show the installed WTF version")
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {
		s.Command = c.CommandPath()
		s.Data = helpData(c)
		if e := s.emit(nil); e != nil {
			_, _ = fmt.Fprintln(errOut, e)
		}
	})
	root.RunE = func(c *cobra.Command, _ []string) error {
		if showVersion {
			s.Command = "wtf version"
			s.Data = map[string]string{"version": Version}
			return nil
		}
		s.Command = c.CommandPath()
		s.Data = helpData(c)
		return nil
	}
	var from, message string
	n := s.command("new PATH [-- HOOK_ARGS...]", "Create or prepare a workspace at a destination path", "Bare names resolve beside the registered main repository, even from another workspace or subdirectory, without a repository prefix or suffix. Explicit relative paths resolve from the current directory; absolute paths also work. An existing JJ workspace root is registered and prepared; a missing path creates an owned JJ workspace at that exact location, named after its basename. --from defaults to trunk() and only applies to creation. Existing directories that are not workspace roots are rejected. Both modes run the init override; only creation runs the new override, which may block before mutation. Repeated calls preserve identity, ownership and reusable stages. JSON registration codes are initialized or already_initialized. Project arguments after -- keep their case. Examples: wtf new .; wtf new agent-a -f @ -- --compose.", pathHookArgs, func(c *cobra.Command, args []string) (any, error) {
		options := core.NewWorkspaceOptions{}
		if c.Flags().Changed("message") {
			options.Message = &message
		}
		return app.NewWorkspaceWithOptions(c.Context(), args[0], from, options, args[1:]...)
	})
	n.Long += " New workspaces copy .wtf/config.toml, schema.json, hooks/ and tasks/ from the invoking workspace before initialization and creation callbacks, with independent files and local ignore rules. Local artifacts and runtime state are excluded. Existing-workspace preparation preserves its configuration."
	n.Flags().StringVarP(&from, "from", "f", "trunk()", "JJ parent revision when creating a missing destination")
	n.Flags().StringVarP(&message, "message", "m", "", "Workspace description shown below the selected row in list and remove; empty clears it")
	root.AddCommand(n)
	var changedMessage string
	change := s.command("change [NAME]", "Change a workspace message", "Replace the message on a registered workspace by name or ID in the current repository. Omit NAME to change the current workspace, including from a subdirectory. --message replaces the description; an empty string clears it. Without a requested change, show help without modifying anything or running hooks. Does not create or prepare workspaces. Examples: wtf change testing-msg -m \"Picking this up tomorrow\"; wtf change testing-msg -m \"\"; wtf change -m \"Picking this up tomorrow\".", cobra.MaximumNArgs(1), func(c *cobra.Command, args []string) (any, error) {
		if !c.Flags().Changed("message") {
			return helpData(c), nil
		}
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return app.ChangeWorkspace(c.Context(), name, changedMessage)
	})
	change.Flags().StringVarP(&changedMessage, "message", "m", "", "Replace the workspace description; empty clears it")
	root.AddCommand(change)
	setup := s.command("setup", "Create local WTF configuration and refresh the agent guide", "Create .wtf in the current directory with config.toml, the generated editor schema, a self-contained AGENTS.md reference, its template checksum, and completely empty hooks and tasks directories. Its .gitignore contains only * so all scaffold files stay local and untracked. Config/schema edits are preserved; repeated setup fills in missing files and refreshes guides matching the previously installed checksum. Customized or unknown guides are preserved with an update-available report. The guide and checksum are inherited by newly created workspaces. Uses the installed binary's embedded template without a network lookup. Does not run hooks, install dependencies or register a workspace. No repository is required. Use setup teardown to remove the local .wtf directory after a yes/no confirmation. Examples: wtf setup; wtf setup --json; wtf setup teardown.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) {
		schema, err := ConfigurationSchema(root)
		if err != nil {
			return nil, err
		}
		return app.Setup(c.Context(), schema)
	})
	setup.AddCommand(s.command("teardown", "Remove local WTF configuration after confirmation", "Remove the entire .wtf directory and its contents from the current directory after an interactive yes/no confirmation, defaulting to No. Works even with broken configuration. Does not run hooks or change workspace registration, source outside .wtf, or shared state. Symlink targets are never removed. JSON and redirected output report confirmation_required without deleting anything; a missing .wtf is a successful no-op. Example: wtf setup teardown.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) {
		return s.teardownSetup(c.Context())
	}))
	root.AddCommand(setup)
	var global, here, all bool
	list := s.command("list [NAME]", "Browse and switch workspaces", "Browse active workspaces with arrow keys; Enter selects, Esc cancels. The view paginates to your terminal size. list and ls are the same command. An optional NAME resolves a workspace directly.\n\nInside a repository, show its workspaces; from a parent directory, show repositories beneath it. --global includes all registered repositories, --all includes unavailable and removed records.\n\n--hook adds local stages, not CI. --pr adds GitHub PR status; each invocation performs a bounded live lookup. JSON and redirected output never prompt. With shell integration, selection changes your directory.\n\nExamples:\n  wtf list\n  wtf ls -p\n  wtf ls --all --json", cobra.MaximumNArgs(1), func(c *cobra.Command, args []string) (any, error) {
		rows, e := app.List(c.Context(), global, here)
		if e != nil {
			return rows, e
		}
		if !all {
			active := make([]core.WorkspaceView, 0, len(rows))
			for _, row := range rows {
				if row.Status != "removed" && (row.Exists || len(args) == 1) {
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
		if s.RefreshPRs && !s.ShowPRs && !s.ShowChecks {
			return nil, core.Fail("invalid_arguments", "--refresh requires --pr or --checks")
		}
		if (s.ShowPRs || s.ShowChecks) && !s.browseInteractive() {
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
	list.Aliases = []string{"ls", "sw"}
	list.Flags().BoolVar(&global, "global", false, "List all registered repositories")
	list.Flags().BoolVar(&here, "here", false, "Only the current managed workspace")
	list.Flags().BoolVarP(&all, "all", "a", false, "Include unavailable and removed workspace records; available workspaces are shown by default")
	list.Flags().BoolVar(&s.SelectPath, "select", false, "Return only the selected path; render the picker on stderr for shell navigation")
	list.Flags().BoolVarP(&s.ShowStages, "hook", "k", false, "Show local hook stages (successful/recorded), not GitHub CI")
	list.Flags().BoolVarP(&s.ShowPRs, "pr", "p", true, "Show PRs with Linear-first review links and GitHub fallback")
	list.Flags().BoolVarP(&s.ShowChecks, "checks", "c", true, "Show GitHub CI check counts for the published PR head")
	list.Flags().BoolVar(&s.RefreshPRs, "refresh", false, "Request live PR metadata (already automatic with --pr)")
	list.MarkFlagsMutuallyExclusive("global", "here")
	root.AddCommand(list)
	rm := s.command("remove [NAME]", "Select and remove workspaces and their owned resources", "Open an inline multi-select cleanup picker. Space toggles a workspace; f toggles force mode to discard remaining source changes; a selects clean workspaces verified in main/master; Enter reviews, then confirms removal. Current and adopted workspaces are protected. Changes outside main/master block removal unless force mode is enabled in the picker. Integration uses local JJ ancestry or a fresh merged GitHub PR with an exact matching head and main/master base. Local bookmarks and remote bookmarks reflect their last fetched state. Normal JJ snapshotting captures on-disk changes during inspection.\n\nAn explicit NAME removes one workspace directly. JSON and redirected output with no NAME report cleanup candidates without removing them. Teardown hooks and identity/resource checks are revalidated for each removal. Failures retain cleanup state.\n\nExamples:\n  wtf remove\n  wtf rm agent-a --json\n  wtf remove --json", cobra.MaximumNArgs(1), func(c *cobra.Command, args []string) (any, error) { return s.remove(c.Context(), args) })
	rm.Aliases = []string{"rm"}
	root.AddCommand(rm)
	root.AddCommand(s.command("run TASK [ARGS...]", "Run a project task", "Run an argv-based TOML task in the current managed workspace, recording its result. Arguments after -- are forwarded literally. Tasks do not reuse results by default. Example: wtf run test -- tests/auth.", cobra.MinimumNArgs(1), func(c *cobra.Command, args []string) (any, error) { return app.Task(c.Context(), args[0], args[1:]) }))
	doctor := s.command("doctor", "Check configuration, tools and workspace health", "Report configuration, tool readiness and workspace health independently, including before workspace registration. Parse strict TOML and compile the selected Lua entrypoint without executing it. Check JJ identity, GitHub authentication and the repository's Codex Linear MCP connection. Inspect retained workspace stages, claims and identity. Checks are bounded; authentication may contact GitHub and Linear. Does not run hooks, register workspaces, snapshot JJ state or repair anything. Invalid configuration, required JJ prerequisites or unhealthy workspace state fail; unregistered workspaces and optional GitHub/Linear warnings do not. Example: wtf doctor --json.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) { return app.Doctor(c.Context(), s.DefaultsError) })
	doctor.Aliases = []string{"doc"}
	doctor.Flags().Bool("here", true, "Inspect the current managed workspace")
	root.AddCommand(doctor)
	hooks := s.group("hook", "Inspect or invoke named hooks")
	hooks.AddCommand(s.command("list", "List configured hooks and overrides", "Load the hook module and list names. No hook function is invoked. Module declarations should be side-effect free. Example: wtf hook list --json.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) { return app.Hooks(c.Context()) }))
	hooks.AddCommand(s.command("run NAME [-- HOOK_ARGS...]", "Run a named hook", "Invoke an explicitly registered project hook under its workspace lock. Arguments after -- reach request.args unchanged. Records stages and stops on failure. Example: wtf hook run test -- --verbose.", namedHookArgs, func(c *cobra.Command, args []string) (any, error) { return app.Hook(c.Context(), args[0], args[1:]...) }))
	root.AddCommand(hooks)
	ports := s.group("ports", "Manage workspace port claims")
	ports.AddCommand(s.command("list", "List current workspace port claims", "Read claims, without checking listeners or choosing ports. Registry claims are not socket reservations. Example: wtf ports list --json.", cobra.NoArgs, func(c *cobra.Command, _ []string) (any, error) {
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
	root.AddCommand(s.command("version", "Show installed version", "Show the installed WTF version. Example: wtf version --json.", cobra.NoArgs, func(_ *cobra.Command, _ []string) (any, error) { return map[string]string{"version": Version}, nil }))
	root.CompletionOptions.DisableDefaultCmd = true
	s.addCompletion()
	for _, command := range root.Commands() {
		switch command.Name() {
		case "list", "new", "change", "remove":
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

// Execute runs a command and returns its process exit code.
func (s *Session) Execute(ctx context.Context, args []string) int {
	if len(args) > 0 && (strings.EqualFold(args[0], "__complete") || strings.EqualFold(args[0], "__completeNoDesc")) {
		return s.complete(ctx, args[1:])
	}
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

// Execute runs a command and returns its process exit code.
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
