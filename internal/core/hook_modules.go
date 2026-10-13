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

type hookPackage struct {
	Root, Entry   string
	CreationHooks []string
	Modules       []string
}

var moduleName = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*$`)
var hookOperations = map[string]bool{"init": true, "new": true, "remove": true}

// Explicit TOML module lists or the selected entrypoint register behavior.
// Unlisted files never register hooks or override operations.
func projectHooks(root string, config Config) (hookPackage, error) {
	modules := config.hookModules()
	if len(modules) > 0 {
		if config.Lua != "" {
			return hookPackage{}, Fail("invalid_config", "Use declarative hook lists or a lua entrypoint, not both")
		}
		for _, name := range modules {
			if !moduleName.MatchString(name) || name == "wtf" || strings.HasPrefix(name, "wtf.") {
				return hookPackage{}, Fail("invalid_config", "Expected a project hook module name, such as hello_world: "+name)
			}
		}
		path, err := relative(filepath.Join(root, ".wtf"), "hooks")
		if err != nil {
			return hookPackage{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return hookPackage{}, err
		}
		if !info.IsDir() {
			return hookPackage{}, Fail("invalid_path", "Expected .wtf/hooks to be a directory")
		}
		return hookPackage{Root: path, CreationHooks: config.New.Hooks, Modules: modules}, nil
	}
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
		if os.IsNotExist(e) {
			if len(config.hookModules()) > 0 {
				return Fail("hook_missing", "Configured hooks require a .wtf/hooks directory")
			}
			return Fail("hook_missing", "Configured Lua hook entrypoint is missing: "+config.Lua)
		}
		return e
	}
	if pkg.Root == "" {
		return nil
	}
	L := newLua(context.Background())
	defer L.Close()
	paths := []string{}
	if pkg.Entry != "" {
		paths = append(paths, pkg.Entry)
	}
	for _, name := range pkg.Modules {
		path, err := relative(pkg.Root, strings.ReplaceAll(name, ".", string(os.PathSeparator))+".lua")
		if os.IsNotExist(err) {
			return Fail("hook_missing", "Configured hook is missing: "+name)
		}
		if err != nil {
			return err
		}
		paths = append(paths, path)
	}
	for _, path := range paths {
		if _, e = L.LoadFile(path); e != nil {
			return Fail("invalid_hook", e.Error())
		}
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
	if pkg.Root == "" {
		return nil
	}
	r.ModuleRoot = pkg.Root
	r.Modules = map[string]lua.LValue{}
	r.Loading = map[string]bool{}
	if err := r.installLuaPackage(); err != nil {
		return err
	}
	r.L.SetGlobal("require", r.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		if !moduleName.MatchString(name) {
			return r.raise(L, Fail("invalid_module", "Use a project module name such as lib.compose"))
		}
		if value, ok := r.Modules[name]; ok {
			L.Push(value)
			return 1
		}
		if name == "wtf" || strings.HasPrefix(name, "wtf.") {
			return r.raise(L, Fail("invalid_module", "Unknown WTF module: "+name))
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
	if pkg.Entry == "" {
		err := r.registerCreationHooks(pkg.Modules)
		r.CreationHooks = append([]string{}, pkg.CreationHooks...)
		return err
	}
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

func (r *Runtime) registerCreationHooks(names []string) error {
	L := r.L
	hooks := L.NewTable()
	for _, name := range names {
		if err := L.CallByParam(lua.P{Fn: L.GetGlobal("require"), NRet: 1, Protect: true}, lua.LString(name)); err != nil {
			return r.operationError(err)
		}
		value := L.Get(-1)
		L.Pop(1)
		if value.Type() != lua.LTFunction {
			return Fail("invalid_hook", "Hook must return a function(event): "+name)
		}
		hooks.RawSetString(name, value)
	}
	module, ok := r.Modules["wtf"].(*lua.LTable)
	if !ok {
		return Fail("invalid_host_api", "WTF package is unavailable")
	}
	options := L.NewTable()
	options.RawSetString("hooks", hooks)
	if err := L.CallByParam(lua.P{Fn: module.RawGetString("setup"), NRet: 1, Protect: true}, options); err != nil {
		return r.operationError(err)
	}
	registration, ok := L.Get(-1).(*lua.LTable)
	L.Pop(1)
	if !ok {
		return Fail("invalid_host_api", "WTF setup must return a registration table")
	}
	var err error
	r.Hooks, err = checkSection(L, registration, "hooks")
	return err
}

func (r *Runtime) runCreationHooks(name, from string, args []string) error {
	request := r.request(name, from, args)
	request.RawSetString("command", lua.LString("new"))
	request.RawSetString("surface", lua.LString("new"))
	request.RawSetString("trigger", lua.LString("after"))
	result, err := luaResult(r.L, r.Workspace)
	if err != nil {
		return err
	}
	request.RawSetString("result", result)
	for _, hook := range r.CreationHooks {
		if err := r.runHook(hook, request); err != nil {
			return err
		}
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
