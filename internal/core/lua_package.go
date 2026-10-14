package core

import (
	_ "embed"

	lua "github.com/yuin/gopher-lua"
)

//go:embed lua/wtf.lua
var luaPackageSource string

// installLuaPackage exposes the existing host primitives through cached modules.
// It does not register any project callback or dispatch an operation.
func (r *Runtime) installLuaPackage() error {
	L := r.L
	module := L.NewTable()
	module.RawSetString("api_version", lua.LNumber(1))
	module.RawSetString("ctx", r.CTX)
	module.RawSetString("notify", L.NewFunction(func(L *lua.LState) int {
		if err := r.App.Notify(L.CheckString(1), L.OptString(2, "info")); err != nil {
			return r.raise(L, err)
		}
		return 0
	}))
	for _, spec := range []struct {
		name    string
		methods map[string]string
	}{
		{"fs", map[string]string{"exists": "exists"}},
		{"exec", map[string]string{"check": "check", "stage": "stage"}},
		{"policy", map[string]string{"require": "require", "block": "block"}},
		{"resources", map[string]string{"list": "claims", "claim": "claim", "release": "release", "claim_port": "claim_port", "listening": "listening"}},
		{"hooks", map[string]string{"run": "hook", "has": "has_hook"}},
	} {
		namespace := L.NewTable()
		for name, method := range spec.methods {
			native, ok := r.CTX.RawGetString(method).(*lua.LFunction)
			if !ok || native.GFunction == nil {
				return Fail("invalid_host_api", "Missing host primitive: "+method)
			}
			namespace.RawSetString(name, L.NewFunction(func(L *lua.LState) int {
				L.Insert(r.CTX, 1)
				return native.GFunction(L)
			}))
		}
		module.RawSetString(spec.name, namespace)
		r.Modules["wtf."+spec.name] = namespace
	}
	if err := r.installSubstrate(module); err != nil {
		return err
	}
	validate := L.NewFunction(func(L *lua.LState) int {
		options, ok := L.Get(1).(*lua.LTable)
		if !ok {
			return r.raise(L, Fail("invalid_hook", "wtf.setup expects an options table"))
		}
		var unexpected string
		var unknown bool
		options.ForEach(func(k, _ lua.LValue) {
			if k.String() != "hooks" && k.String() != "overrides" {
				unexpected = k.String()
				unknown = true
			}
		})
		if unknown {
			return r.raise(L, Fail("invalid_hook", "Unknown wtf.setup option: "+unexpected))
		}
		for _, name := range []string{"hooks", "overrides"} {
			section, err := checkSection(L, options, name)
			if err != nil {
				return r.raise(L, err)
			}
			if name == "overrides" {
				var unsupported string
				section.ForEach(func(k, _ lua.LValue) {
					if !hookOperations[k.String()] {
						unsupported = k.String()
					}
				})
				if unsupported != "" {
					return r.raise(L, Fail("invalid_hook", "Unsupported operation override: "+unsupported))
				}
			}
		}
		return 0
	})
	factory, err := L.LoadString(luaPackageSource)
	if err != nil {
		return Fail("invalid_host_api", err.Error())
	}
	if err = L.CallByParam(lua.P{Fn: factory, NRet: 1, Protect: true}); err != nil {
		return r.operationError(err)
	}
	constructor := L.Get(-1)
	L.Pop(1)
	if err = L.CallByParam(lua.P{Fn: constructor, NRet: 1, Protect: true}, module, validate); err != nil {
		return r.operationError(err)
	}
	r.Modules["wtf"] = L.Get(-1)
	L.Pop(1)
	return nil
}
