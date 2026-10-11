package core

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Runtime owns a restricted Lua hook interpreter and workspace context.
type Runtime struct {
	App        *App
	Repo       Repo
	Workspace  Workspace
	L          *lua.LState
	Context    context.Context
	Cancel     context.CancelFunc
	CTX        *lua.LTable
	Hooks      *lua.LTable
	Overrides  *lua.LTable
	ModuleRoot string
	Modules    map[string]lua.LValue
	Loading    map[string]bool
	Request    *lua.LTable
}

func (a *App) runtime(ctx context.Context, r Repo, w Workspace) (*Runtime, error) {
	c, e := LoadConfig(r.Root)
	if e != nil {
		return nil, e
	}
	hookCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	L := newLua(hookCtx)
	rt := &Runtime{App: a, Repo: r, Workspace: w, L: L, Context: hookCtx, Cancel: cancel}
	complete := false
	defer func() {
		if !complete {
			cancel()
		}
	}()
	rt.CTX = L.NewTable()
	rt.Hooks = L.NewTable()
	rt.Overrides = L.NewTable()
	rt.install()
	rt.Bind(w)
	project := L.NewTable()
	project.RawSetString("path", lua.LString(r.Root))
	if state, err := r.Load(); err == nil {
		project.RawSetString("id", lua.LString(state.RepositoryID))
	}
	rt.CTX.RawSetString("project", project)
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		for i := 1; i <= L.GetTop(); i++ {
			_, _ = fmt.Fprint(a.Out, L.Get(i).String(), " ")
		}
		_, _ = fmt.Fprintln(a.Out)
		return 0
	}))
	pkg, err := projectHooks(r.Root, c)
	if err == nil {
		err = rt.registerPackage(pkg)
	}
	if err != nil {
		L.Close()
		return nil, err
	}
	complete = true
	return rt, nil
}

// Close cancels hook execution and closes the Lua interpreter.
func (r *Runtime) Close() { r.Cancel(); r.L.Close() }

// Bind sets the current registered workspace in the hook context.
func (r *Runtime) Bind(w Workspace) {
	r.Workspace = w
	if project, ok := r.CTX.RawGetString("project").(*lua.LTable); ok {
		if state, err := r.Repo.Load(); err == nil {
			project.RawSetString("id", lua.LString(state.RepositoryID))
		}
	}
	if w.ID == "" {
		r.CTX.RawSetString("workspace", lua.LNil)
		return
	}
	r.CTX.RawSetString("workspace", luaWorkspace(r.L, w))
}
func (r *Runtime) raise(state *lua.LState, e error) int { return luaError(state, e) }
func (r *Runtime) operationError(e error) error {
	if cause := hostError(e); cause != nil {
		return cause
	}
	if r.Context.Err() != nil {
		return Fail("hook_cancelled", r.Context.Err().Error())
	}
	return Fail("hook_failed", e.Error())
}
func (r *Runtime) request(name, from string, args []string) *lua.LTable {
	request := r.L.NewTable()
	request.RawSetString("name", lua.LString(name))
	request.RawSetString("from", lua.LString(from))
	argv := r.L.NewTable()
	for _, arg := range args {
		argv.Append(lua.LString(arg))
	}
	request.RawSetString("args", argv)
	return request
}

// Run invokes a registered project hook.
func (r *Runtime) Run(name string, args ...string) error {
	return r.runHook(name, r.request(r.Workspace.Name, "", args))
}
func (r *Runtime) runHook(name string, request *lua.LTable) error {
	fn := r.Hooks.RawGetString(name)
	if fn == lua.LNil {
		return Fail("hook_missing", "No hook named "+name)
	}
	previous := r.Request
	r.Request = request
	defer func() { r.Request = previous }()
	if e := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, r.CTX, request); e != nil {
		return r.operationError(e)
	}
	return nil
}

// Override invokes an explicit lifecycle override or its default operation.
func (r *Runtime) Override(op, name, from string, def func(string, string) (Workspace, error), args ...string) error {
	fn := r.Overrides.RawGetString(op)
	if fn == lua.LNil {
		_, e := def(name, from)
		return e
	}
	request := r.request(name, from, args)
	r.Request = request
	defaultFn := r.L.NewFunction(func(L *lua.LState) int {
		req := L.CheckTable(1)
		n := asString(req.RawGetString("name"))
		f := asString(req.RawGetString("from"))
		if (op == "remove" || op == "init") && n != name {
			return r.raise(L, Fail("identity_mismatch", "Init/remove overrides cannot change the target identity"))
		}
		w, e := def(n, f)
		if e != nil {
			return r.raise(L, e)
		}
		L.Push(luaWorkspace(L, w))
		return 1
	})
	if e := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, r.CTX, request, defaultFn); e != nil {
		return r.operationError(e)
	}
	return nil
}
func (r *Runtime) requireWorkspace(_ *lua.LState) error {
	if r.Workspace.ID == "" {
		return Fail("workspace_required", "Call default creation before workspace-specific stages")
	}
	return nil
}
func (r *Runtime) install() {
	L := r.L
	r.CTX.RawSetString("stage", L.NewFunction(func(L *lua.LState) int {
		if e := r.requireWorkspace(L); e != nil {
			return r.raise(L, e)
		}
		name := L.CheckString(2)
		argv, e := tableStrings(L, L.Get(3))
		if e != nil {
			return r.raise(L, e)
		}
		o := StageOptions{}
		if opts, ok := L.Get(4).(*lua.LTable); ok {
			if v := opts.RawGetString("inputs"); v != lua.LNil {
				o.Inputs, e = tableStrings(L, v)
				if e != nil {
					return r.raise(L, e)
				}
			}
			if v := opts.RawGetString("check"); v != lua.LNil {
				o.Check, e = tableStrings(L, v)
				if e != nil {
					return r.raise(L, e)
				}
			}
			o.Reuse = lua.LVAsBool(opts.RawGetString("reuse"))
			if v := opts.RawGetString("timeout_seconds"); v != lua.LNil {
				n, ok := v.(lua.LNumber)
				if !ok || math.Trunc(float64(n)) != float64(n) || n < 1 || n > 3600 {
					return r.raise(L, Fail("invalid_timeout", "Timeout must be an integer from 1 to 3600"))
				}
				o.TimeoutSeconds = int(n)
			}
			if v := opts.RawGetString("env"); v != lua.LNil {
				t, ok := v.(*lua.LTable)
				if !ok {
					return r.raise(L, Fail("invalid_hook", "env must be a string map"))
				}
				o.Env = map[string]string{}
				var bad bool
				t.ForEach(func(k, v lua.LValue) {
					if k.Type() != lua.LTString || v.Type() != lua.LTString {
						bad = true
					} else {
						o.Env[k.String()] = v.String()
					}
				})
				if bad {
					return r.raise(L, Fail("invalid_hook", "env must contain string keys and values"))
				}
			}
		}
		st, e := r.App.Stage(r.Context, r.Repo, r.Workspace, name, argv, o)
		if e != nil {
			return r.raise(L, e)
		}
		t := L.NewTable()
		t.RawSetString("status", lua.LString(st.Status))
		t.RawSetString("reused", lua.LBool(st.Reused))
		L.Push(t)
		return 1
	}))
	r.CTX.RawSetString("hook", L.NewFunction(func(L *lua.LState) int {
		request := r.Request
		if L.Get(3) != lua.LNil {
			request = L.CheckTable(3)
		}
		if request == nil {
			request = r.request(r.Workspace.Name, "", nil)
		}
		if e := r.runHook(L.CheckString(2), request); e != nil {
			return r.raise(L, e)
		}
		return 0
	}))
	r.CTX.RawSetString("has_hook", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(r.Hooks.RawGetString(L.CheckString(2)) != lua.LNil))
		return 1
	}))
	r.CTX.RawSetString("check", L.NewFunction(func(L *lua.LState) int {
		argv, e := tableStrings(L, L.Get(2))
		if e != nil {
			return r.raise(L, e)
		}
		dir := r.Repo.Root
		if r.Workspace.ID != "" {
			dir = r.Workspace.Path
		}
		e = r.App.Exec(r.Context, dir, argv, nil, 30*time.Second, true)
		if r.Context.Err() != nil {
			return r.raise(L, Fail("hook_cancelled", r.Context.Err().Error()))
		}
		L.Push(lua.LBool(e == nil))
		return 1
	}))
	r.CTX.RawSetString("require", L.NewFunction(func(L *lua.LState) int {
		if !L.CheckBool(2) {
			return r.raise(L, Fail("hook_blocked", L.CheckString(3)))
		}
		return 0
	}))
	r.CTX.RawSetString("block", L.NewFunction(func(L *lua.LState) int { return r.raise(L, Fail("hook_blocked", L.CheckString(2))) }))
	r.CTX.RawSetString("exists", L.NewFunction(func(L *lua.LState) int {
		dir := r.Repo.Root
		if r.Workspace.ID != "" {
			dir = r.Workspace.Path
		}
		_, err := relative(dir, L.CheckString(2))
		if err != nil && !os.IsNotExist(err) {
			return r.raise(L, err)
		}
		L.Push(lua.LBool(err == nil))
		return 1
	}))
	r.CTX.RawSetString("claims", L.NewFunction(func(L *lua.LState) int {
		if e := r.requireWorkspace(L); e != nil {
			return r.raise(L, e)
		}
		claims, e := r.Repo.Claims(r.Workspace)
		if e != nil {
			return r.raise(L, e)
		}
		result := L.NewTable()
		for _, claim := range claims {
			t := L.NewTable()
			t.RawSetString("kind", lua.LString(claim.Kind))
			t.RawSetString("name", lua.LString(claim.Name))
			t.RawSetString("value", lua.LString(claim.Value))
			result.Append(t)
		}
		L.Push(result)
		return 1
	}))
	r.CTX.RawSetString("claim", L.NewFunction(func(L *lua.LState) int {
		if e := r.requireWorkspace(L); e != nil {
			return r.raise(L, e)
		}
		c, e := r.Repo.Claim(r.Context, r.Workspace, L.CheckString(2), L.CheckString(3), L.CheckString(4))
		if e != nil {
			return r.raise(L, e)
		}
		L.Push(lua.LString(c.Value))
		return 1
	}))
	r.CTX.RawSetString("release", L.NewFunction(func(L *lua.LState) int {
		if e := r.requireWorkspace(L); e != nil {
			return r.raise(L, e)
		}
		if e := r.Repo.Release(r.Context, r.Workspace, L.CheckString(2), L.CheckString(3)); e != nil {
			return r.raise(L, e)
		}
		return 0
	}))
	r.CTX.RawSetString("claim_port", L.NewFunction(func(L *lua.LState) int {
		if e := r.requireWorkspace(L); e != nil {
			return r.raise(L, e)
		}
		name := L.CheckString(2)
		n := L.CheckNumber(3)
		if math.Trunc(float64(n)) != float64(n) || n < 1 || n > 65535 {
			return r.raise(L, Fail("invalid_port", "Port must be an integer from 1 to 65535"))
		}
		port := int(n)
		_, e := r.Repo.Claim(r.Context, r.Workspace, "port", name, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if e != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(ErrorCode(e)))
			return 2
		}
		L.Push(lua.LNumber(port))
		return 1
	}))
	r.CTX.RawSetString("listening", L.NewFunction(func(L *lua.LState) int {
		n := L.CheckNumber(2)
		if math.Trunc(float64(n)) != float64(n) || n < 1 || n > 65535 {
			return r.raise(L, Fail("invalid_port", "Invalid TCP port"))
		}
		d := net.Dialer{Timeout: 200 * time.Millisecond}
		conn, e := d.DialContext(r.Context, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(n))))
		if e == nil {
			_ = conn.Close()
		}
		L.Push(lua.LBool(e == nil))
		return 1
	}))
}

// Hooks lists registered hooks and overrides without invoking them.
func (a *App) Hooks(ctx context.Context) (map[string][]string, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return nil, e
	}
	rt, e := a.runtime(ctx, r, Workspace{})
	if e != nil {
		return nil, e
	}
	defer rt.Close()
	result := map[string][]string{"hooks": {}, "overrides": {}}
	for name, t := range map[string]*lua.LTable{"hooks": rt.Hooks, "overrides": rt.Overrides} {
		t.ForEach(func(k, _ lua.LValue) { result[name] = append(result[name], k.String()) })
		sort.Strings(result[name])
	}
	return result, nil
}
