package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// HookInvocation identifies a parsed execution command without changing its argv.
type HookInvocation struct {
	Command string
	Args    []string
	Path    string
	From    string
}

// InvokeHooks wraps a command with explicit invocation and surface phases.
// Parsing, help, diagnostics and recovery commands are filtered by the CLI.
func (a *App) InvokeHooks(ctx context.Context, invocation HookInvocation, operation func() (any, error)) (any, error) {
	r, discoveryErr := Discover(ctx, a.Cwd)
	if discoveryErr != nil {
		if ctx.Err() != nil {
			return nil, Fail("command_cancelled", ctx.Err().Error())
		}
		r = Repo{Root: a.Cwd}
	}
	cfg, err := LoadConfig(r.Root)
	if err != nil {
		return nil, err
	}
	surface := strings.Fields(invocation.Command)
	name := ""
	if len(surface) > 0 {
		name = surface[0]
	}
	if name == "remove" && discoveryErr != nil {
		// Parent-scoped removal invokes each selected repository's lifecycle
		// separately, after unambiguous target resolution.
		return operation()
	}
	var scope HookTriggers
	switch name {
	case "new":
		// Existing JJ destinations are preparation, not creation. The legacy
		// hooks shorthand still runs inside native creation, under its lock.
		if _, err := os.Stat(filepath.Join(invocation.Path, ".jj")); os.IsNotExist(err) {
			scope = cfg.New
			scope.Hooks = nil
		}
	case "list":
		scope = cfg.List
	case "remove":
		scope = cfg.Remove
	}
	global := cfg.Invocation
	if len(scope.phases()) == 0 && len(global.phases()) == 0 {
		return operation()
	}
	var workspace Workspace
	if r.Store != "" {
		workspace, _ = r.Workspace(ctx)
	}
	var removalTarget Workspace
	if name == "remove" && len(invocation.Args) > 0 && r.Store != "" {
		removalTarget, _ = r.Select(invocation.Args[0])
	}
	rt, err := a.runtimeWithConfig(ctx, r, workspace, cfg)
	if err != nil {
		return nil, err
	}
	defer rt.Close()
	executionCtx := rt.Context
	args := invocation.Args
	target := workspace.Name
	if len(args) > 0 {
		target = args[0]
	}
	if name == "new" {
		target = filepath.Base(invocation.Path)
		if len(args) > 0 {
			args = args[1:]
		}
	}
	request := rt.request(target, invocation.From, args)
	request.RawSetString("command", lua.LString(invocation.Command))
	if invocation.Path != "" {
		request.RawSetString("path", lua.LString(invocation.Path))
	}
	var data any
	current := workspace
	phase := func(surface, trigger string, modules []string, exhaustive bool) error {
		if len(modules) == 0 {
			return nil
		}
		phaseCtx := executionCtx
		if trigger == "failed" || trigger == "finally" {
			var cancel context.CancelFunc
			phaseCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
		}
		rt.Context = phaseCtx
		rt.L.SetContext(phaseCtx)
		rt.Bind(current)
		if surface == "remove" && removalTarget.ID != "" {
			if state, loadErr := r.Load(); loadErr == nil {
				if latest, exists := state.Workspaces[removalTarget.ID]; exists {
					removalTarget = latest
				}
			}
			rt.Bind(removalTarget)
		}
		request.RawSetString("surface", lua.LString(surface))
		request.RawSetString("trigger", lua.LString(trigger))
		request.RawSetString("error", lua.LNil)
		if err != nil {
			problem := rt.L.NewTable()
			problem.RawSetString("code", lua.LString(ErrorCode(err)))
			problem.RawSetString("message", lua.LString(err.Error()))
			request.RawSetString("error", problem)
		}
		result, conversionErr := luaResult(rt.L, data)
		if conversionErr != nil {
			return conversionErr
		}
		request.RawSetString("result", result)
		if rt.Workspace.ID != "" && r.Store != "" {
			unlock, lockErr := r.LockWorkspace(phaseCtx, rt.Workspace.ID)
			if lockErr != nil {
				return lockErr
			}
			defer unlock()
		}
		var failure error
		for _, module := range modules {
			if hookErr := rt.runHook(module, request); hookErr != nil {
				failure = errors.Join(failure, hookErr)
				if !exhaustive {
					break
				}
			}
		}
		return failure
	}
	err = phase("invocation", "before", global.Before, false)
	if err == nil {
		err = phase(name, "before", scope.Before, false)
	}
	if err == nil {
		data, err = operation()
		if created, ok := data.(Initialization); ok && created.ID != "" {
			current = created.Workspace
		}
	}
	if err == nil {
		err = phase(name, "after", append(append([]string{}, scope.Hooks...), scope.After...), false)
	}
	if err == nil {
		err = phase("invocation", "after", append(append([]string{}, global.Hooks...), global.After...), false)
	}
	if err != nil {
		err = errors.Join(err, phase(name, "failed", scope.Failed, true))
		err = errors.Join(err, phase("invocation", "failed", global.Failed, true))
	}
	err = errors.Join(err, phase(name, "finally", scope.Finally, true))
	err = errors.Join(err, phase("invocation", "finally", global.Finally, true))
	if err != nil && name == "new" {
		if created, ok := data.(Initialization); ok && created.ID != "" {
			persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = r.Update(persist, created.ID, func(w *Workspace) { w.Status = "hook_failed"; w.LastError = err.Error() })
			if retained, selectErr := r.Select(created.ID); selectErr == nil {
				created.Workspace = retained
				data = created
			}
		}
	}
	return data, err
}

func luaResult(state *lua.LState, data any) (lua.LValue, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, Fail("invalid_hook_result", "Command result cannot be exposed to hooks: "+err.Error())
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return luaJSON(state, decoded), nil
}

func luaJSON(state *lua.LState, value any) lua.LValue {
	switch value := value.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(value)
	case bool:
		return lua.LBool(value)
	case float64:
		return lua.LNumber(value)
	case []any:
		table := state.NewTable()
		for _, item := range value {
			table.Append(luaJSON(state, item))
		}
		return table
	case map[string]any:
		table := state.NewTable()
		for key, item := range value {
			table.RawSetString(key, luaJSON(state, item))
		}
		return table
	default:
		return lua.LNil
	}
}
