package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	lua "github.com/yuin/gopher-lua"
)

// Task defines a project command and its recorded inputs.
type Task struct {
	Command        []string `toml:"command" schema:"Command argv, executed directly without shell interpolation. Additional task arguments are appended literally." schema-required:"true" schema-min-items:"1"`
	Inputs         []string `toml:"inputs" schema:"Project-relative files or directories included in the recorded stage fingerprint. Runtime checks reject escaping paths."`
	TimeoutSeconds int      `toml:"timeout_seconds" schema:"Task timeout in seconds. Omitted or zero uses 120 seconds." schema-minimum:"0" schema-maximum:"3600" schema-default:"0"`
}

// HookTriggers selects explicit modules for lifecycle phases.
type HookTriggers struct {
	Hooks   []string `toml:"hooks" schema:"Compatibility shorthand for after. For new, runs inside native creation; other surfaces run these before explicit after hooks. Modules return function(event), names preserve case, and lists do not inherit. Prefer explicit phase arrays."`
	Before  []string `toml:"before" schema:"Ordered modules run before the operation. A failure blocks the operation. Modules return function(event)."`
	After   []string `toml:"after" schema:"Ordered modules run after a successful operation. A failure stops the sequence and fails the command."`
	Failed  []string `toml:"failed" schema:"Modules run on failure with event.error. All are attempted; they cannot hide the primary error."`
	Finally []string `toml:"finally" schema:"Modules run on success or failure. All are attempted with bounded cleanup time, including after cancellation."`
}

// CreationHooks retains the original Go type name for callers.
type CreationHooks = HookTriggers

func (h HookTriggers) phases() []string {
	var names []string
	for _, phase := range [][]string{h.Hooks, h.Before, h.After, h.Failed, h.Finally} {
		names = append(names, phase...)
	}
	return names
}

func (c Config) hookModules() []string {
	var names []string
	for _, scope := range []HookTriggers{c.New, c.List, c.Remove, c.Invocation} {
		names = append(names, scope.phases()...)
	}
	return names
}

// Config contains project hooks, tasks and command defaults.
type Config struct {
	Version    int                       `toml:"version" schema:"WTF configuration format version." schema-required:"true" schema-const:"1"`
	Lua        string                    `toml:"lua" schema:"Explicit Lua registration module relative to .wtf, such as hooks/init.lua. Filenames never register hooks; the returned hooks/overrides table does. Project modules may be imported with require."`
	New        HookTriggers              `toml:"new" schema:"Hooks around creation of a missing destination. Existing JJ workspace preparation does not run these phases. No registration entrypoint is needed."`
	List       HookTriggers              `toml:"list" schema:"Hooks around the list command, including aliases. after receives the listing or selection as event.result."`
	Remove     HookTriggers              `toml:"remove" schema:"Hooks around the remove command, including aliases. Native removal safety checks remain authoritative."`
	Invocation HookTriggers              `toml:"invocation" schema:"Hooks around execution commands. Inspection, help, version and setup teardown never invoke callbacks. Wraps command-specific phases."`
	Tasks      map[string]Task           `toml:"tasks" schema:"Named project tasks. Tasks use the selected project's configuration rather than inherited command defaults."`
	Commands   map[string]map[string]any `toml:"commands" schema:"Defaults for registered command flags. Precedence is global, outer parent, nearest parent, leaf, then explicit CLI flags. These schema defaults are annotations and do not perform the merge."`
}

func relative(root, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", Fail("invalid_path", "Expected a relative project path")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", Fail("invalid_path", "Path escapes the project")
	}
	p := filepath.Join(root, clean)
	real, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(root, real)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", Fail("invalid_path", "Symlink escapes the project")
	}
	return real, nil
}

// LoadConfig loads and validates the project configuration.
func LoadConfig(root string) (Config, error) {
	c := Config{}
	p := filepath.Join(root, ".wtf", "config.toml")
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return Config{Version: 1, Tasks: map[string]Task{}}, nil
	}
	if e != nil {
		return c, e
	}
	dec := toml.NewDecoder(strings.NewReader(string(b))).DisallowUnknownFields()
	if e = dec.Decode(&c); e != nil {
		return c, Fail("invalid_config", e.Error())
	}
	if c.Version != 1 {
		return c, Fail("invalid_config", "Expected config version = 1")
	}
	for name, t := range c.Tasks {
		if e = CheckName(name); e != nil {
			return c, e
		}
		if len(t.Command) == 0 {
			return c, Fail("invalid_config", "Task "+name+" has no command")
		}
		if t.TimeoutSeconds < 0 || t.TimeoutSeconds > 3600 {
			return c, Fail("invalid_config", "Task timeout must be between 1 and 3600 seconds (or omitted)")
		}
	}
	if e = validateHookPackage(root, c); e != nil {
		return c, e
	}
	return c, nil
}
func newLua(ctx context.Context) *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	lua.OpenBase(L)
	lua.OpenTable(L)
	lua.OpenString(L)
	lua.OpenMath(L)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("collectgarbage", lua.LNil)
	L.SetContext(ctx)
	return L
}
func taskTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 120 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// Validate checks configuration without executing project hooks.
func (a *App) Validate(ctx context.Context) (Config, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Config{}, e
	}
	c, e := LoadConfig(r.Root)
	if e != nil {
		return c, e
	}
	return c, nil
}

// Task runs a configured task in the current workspace.
func (a *App) Task(ctx context.Context, name string, args []string) (Stage, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Stage{}, e
	}
	w, e := r.Workspace(ctx)
	if e != nil {
		return Stage{}, e
	}
	unlock, e := r.LockWorkspace(ctx, w.ID)
	if e != nil {
		return Stage{}, e
	}
	defer unlock()
	c, e := LoadConfig(w.Path)
	if e != nil {
		return Stage{}, e
	}
	t, ok := c.Tasks[name]
	if !ok {
		return Stage{}, Fail("task_missing", "No task named "+name)
	}
	cmd := append(append([]string{}, t.Command...), args...)
	return a.Stage(ctx, r, w, "task-"+name, cmd, StageOptions{Inputs: t.Inputs, TimeoutSeconds: t.TimeoutSeconds})
}
func tableStrings(_ *lua.LState, v lua.LValue) ([]string, error) {
	t, ok := v.(*lua.LTable)
	if !ok {
		return nil, Fail("invalid_hook", "Expected an argv or input array")
	}
	out := []string{}
	var e error
	t.ForEach(func(k, v lua.LValue) {
		if k.Type() != lua.LTNumber || v.Type() != lua.LTString {
			e = Fail("invalid_hook", "Array entries must be strings")
		}
	})
	if e != nil {
		return nil, e
	}
	for i := 1; i <= t.Len(); i++ {
		v := t.RawGetInt(i)
		s, ok := v.(lua.LString)
		if !ok {
			return nil, Fail("invalid_hook", "Array must be contiguous")
		}
		out = append(out, string(s))
	}
	return out, nil
}
func luaWorkspace(state *lua.LState, w Workspace) *lua.LTable {
	t := state.NewTable()
	t.RawSetString("id", lua.LString(w.ID))
	t.RawSetString("name", lua.LString(w.Name))
	t.RawSetString("path", lua.LString(w.Path))
	t.RawSetString("status", lua.LString(w.Status))
	t.RawSetString("message", lua.LString(w.Message))
	return t
}
func asString(v lua.LValue) string {
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// HookPath resolves the configured Lua module within the project.
func (c Config) HookPath(root string) (string, error) {
	if c.Lua == "" {
		return "", Fail("hook_missing", "No Lua hook file configured")
	}
	return relative(filepath.Join(root, ".wtf"), c.Lua)
}
func checkSection(state *lua.LState, t *lua.LTable, key string) (*lua.LTable, error) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return state.NewTable(), nil
	}
	section, ok := v.(*lua.LTable)
	if !ok {
		return nil, Fail("invalid_hook", key+" must be a table")
	}
	var e error
	section.ForEach(func(k, v lua.LValue) {
		if k.Type() != lua.LTString || v.Type() != lua.LTFunction {
			e = Fail("invalid_hook", fmt.Sprintf("%s entries must be named functions", key))
		}
	})
	return section, e
}
