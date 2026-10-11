package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type hookPackage struct{ Root, Entry string }

var moduleName = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*$`)
var hookOperations = map[string]bool{"init": true, "new": true, "remove": true}

// The TOML entrypoint and its returned registration table are authoritative.
// Directory names and filenames never register hooks or override operations.
func projectHooks(root string, config Config) (hookPackage, error) {
	if config.Lua == "" {
		return hookPackage{}, nil
	}
	path, e := config.HookPath(root)
	if e != nil {
		return hookPackage{}, e
	}
	return hookPackage{Root: filepath.Dir(path), Entry: path}, nil
}
func validateHookPackage(root string, config Config) error {
	pkg, e := projectHooks(root, config)
	if e != nil {
		return e
	}
	if pkg.Entry == "" {
		return nil
	}
	L := newLua(context.Background())
	defer L.Close()
	if _, e = L.LoadFile(pkg.Entry); e != nil {
		return Fail("invalid_hook", e.Error())
	}
	return nil
}
func (r *Runtime) loadLua(path string) (lua.LValue, error) {
	fn, e := r.L.LoadFile(path)
	if e != nil {
		return nil, Fail("invalid_hook", e.Error())
	}
	if e = r.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); e != nil {
		return nil, r.operationError(e)
	}
	value := r.L.Get(-1)
	r.L.Pop(1)
	return value, nil
}
func (r *Runtime) registerPackage(pkg hookPackage) error {
	if pkg.Entry == "" {
		return nil
	}
	r.ModuleRoot = pkg.Root
	r.Modules = map[string]lua.LValue{}
	r.Loading = map[string]bool{}
	r.L.SetGlobal("require", r.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		if !moduleName.MatchString(name) {
			return r.raise(L, Fail("invalid_module", "Use a project module name such as lib.compose"))
		}
		if value, ok := r.Modules[name]; ok {
			L.Push(value)
			return 1
		}
		path, e := relative(r.ModuleRoot, strings.ReplaceAll(name, ".", string(os.PathSeparator))+".lua")
		if e != nil {
			return r.raise(L, Fail("invalid_module", "Cannot load project module "+name+": "+e.Error()))
		}
		if r.Loading[path] {
			return r.raise(L, Fail("module_cycle", "Circular module dependency: "+name))
		}
		r.Loading[path] = true
		value, e := r.loadLua(path)
		delete(r.Loading, path)
		if e != nil {
			return r.raise(L, e)
		}
		if value == lua.LNil {
			value = lua.LTrue
		}
		r.Modules[name] = value
		L.Push(value)
		return 1
	}))
	r.Loading[pkg.Entry] = true
	value, e := r.loadLua(pkg.Entry)
	delete(r.Loading, pkg.Entry)
	if e != nil {
		return e
	}
	table, ok := value.(*lua.LTable)
	if !ok {
		return Fail("invalid_hook", "Hook module must return a registration table")
	}
	r.Hooks, e = checkSection(r.L, table, "hooks")
	if e == nil {
		r.Overrides, e = checkSection(r.L, table, "overrides")
	}
	if e != nil {
		return e
	}
	var bad string
	r.Overrides.ForEach(func(k, _ lua.LValue) {
		if !hookOperations[k.String()] {
			bad = k.String()
		}
	})
	if bad != "" {
		return Fail("invalid_hook", "Pilot does not override "+bad)
	}
	return nil
}

// Host errors travel with the Lua error object, so caught errors do not pollute a
// later unrelated failure. Scripts can inspect code/message through __index.
func luaError(state *lua.LState, err error) int {
	object := state.NewUserData()
	object.Value = err
	meta := state.NewTable()
	meta.RawSetString("__tostring", state.NewFunction(func(state *lua.LState) int { state.Push(lua.LString(err.Error())); return 1 }))
	fields := state.NewTable()
	fields.RawSetString("code", lua.LString(ErrorCode(err)))
	fields.RawSetString("message", lua.LString(err.Error()))
	meta.RawSetString("__index", fields)
	state.SetMetatable(object, meta)
	state.Error(object, 1)
	return 0
}
func hostError(err error) error {
	var api *lua.ApiError
	if errors.As(err, &api) {
		if object, ok := api.Object.(*lua.LUserData); ok {
			if cause, ok := object.Value.(error); ok {
				return cause
			}
		}
	}
	return nil
}
