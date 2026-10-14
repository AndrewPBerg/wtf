package core

import lua "github.com/yuin/gopher-lua"

// Parent packages use separate interpreters so equally named modules and imports
// cannot shadow one another. Their filesystem context remains the selected repo.
func (r *Runtime) registerParents(parents []parentHooks) error {
	for _, layer := range parents {
		config := layer.config
		config.packageRoot = layer.root
		parent, err := r.App.runtimeWithConfig(r.Context, r.Repo, r.Workspace, config)
		if err != nil {
			return err
		}
		r.Parents = append(r.Parents, parent)
		var collision error
		parent.Hooks.ForEach(func(key, _ lua.LValue) {
			name := key.String()
			qualified := layer.prefix + name
			if r.Hooks.RawGetString(qualified) != lua.LNil {
				collision = Fail("invalid_hook", "Hook name conflicts with inherited hook: "+qualified)
				return
			}
			r.Hooks.RawSetString(qualified, r.L.NewFunction(func(L *lua.LState) int {
				parent.Context = r.Context
				parent.L.SetContext(r.Context)
				parent.Destination = r.Destination
				parent.Bind(r.Workspace)
				request, err := copyHookRequest(parent.L, L.CheckTable(2))
				if err != nil {
					return r.raise(L, err)
				}
				if err := parent.runHook(name, request); err != nil {
					return r.raise(L, err)
				}
				return 0
			}))
		})
		if collision != nil {
			return collision
		}
	}
	return nil
}

// Requests contain only tables and scalar snapshots, never runtime objects.
func copyHookRequest(state *lua.LState, request *lua.LTable) (*lua.LTable, error) {
	seen := map[*lua.LTable]*lua.LTable{}
	var copyValue func(lua.LValue) (lua.LValue, error)
	copyTable := func(table *lua.LTable) (*lua.LTable, error) {
		if result, ok := seen[table]; ok {
			return result, nil
		}
		result := state.NewTable()
		seen[table] = result
		var failure error
		table.ForEach(func(key, value lua.LValue) {
			if failure != nil {
				return
			}
			key, failure = copyValue(key)
			if failure != nil {
				return
			}
			value, failure = copyValue(value)
			if failure == nil {
				result.RawSet(key, value)
			}
		})
		return result, failure
	}
	copyValue = func(value lua.LValue) (lua.LValue, error) {
		switch value := value.(type) {
		case *lua.LTable:
			return copyTable(value)
		case lua.LString, lua.LNumber, lua.LBool, *lua.LNilType:
			return value, nil
		default:
			return nil, Fail("invalid_hook_request", "Inherited hooks accept only tables and scalar request values")
		}
	}
	return copyTable(request)
}
