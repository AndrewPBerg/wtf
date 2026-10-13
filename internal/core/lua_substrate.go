package core

import (
	"os"
	"path/filepath"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func (r *Runtime) pushValue(L *lua.LState, value any) int {
	result, err := luaResult(L, value)
	if err != nil {
		return r.raise(L, err)
	}
	L.Push(result)
	return 1
}

func parseFileOptions(L *lua.LState, value lua.LValue, allowed ...string) (fileOptions, error) {
	o := fileOptions{}
	t, err := luaOptions(L, value, allowed...)
	if err != nil {
		return o, err
	}
	for _, field := range []struct {
		key  string
		dest *string
	}{{"root", &o.Root}, {"from", &o.From}, {"to", &o.To}, {"symlinks", &o.Symlinks}} {
		*field.dest, err = stringOption(t, field.key, "")
		if err != nil {
			return o, err
		}
	}
	for _, field := range []struct {
		key  string
		dest *bool
	}{{"replace", &o.Replace}, {"recursive", &o.Recursive}, {"follow", &o.Follow}} {
		*field.dest, err = boolOption(t, field.key, false)
		if err != nil {
			return o, err
		}
	}
	if o.Symlinks != "" && o.Symlinks != "preserve" && o.Symlinks != "follow" {
		return o, Fail("invalid_options", "symlinks must be preserve or follow")
	}
	return o, nil
}

func (r *Runtime) fileCall(L *lua.LState, operation string) int {
	index := 2
	allowed := []string{"root"}
	switch operation {
	case "write":
		index = 3
		allowed = append(allowed, "replace")
	case "copy":
		index = 3
		allowed = append(allowed, "from", "to", "replace", "recursive", "symlinks")
	case "symlink":
		index = 3
		allowed = append(allowed, "from", "to", "replace")
	case "exists", "stat":
		allowed = append(allowed, "follow")
	case "remove":
		allowed = append(allowed, "recursive")
	}
	options, err := parseFileOptions(L, L.Get(index), allowed...)
	if err != nil {
		return r.raise(L, err)
	}
	mutate := operation == "write" || operation == "mkdir" || operation == "remove"
	name, err := fileName(L.CheckString(1), mutate)
	if err != nil {
		return r.raise(L, err)
	}
	if err := r.Context.Err(); err != nil {
		return r.raise(L, Fail("hook_cancelled", err.Error()))
	}
	if operation == "copy" || operation == "symlink" {
		destName, err := fileName(L.CheckString(2), true)
		if err != nil {
			return r.raise(L, err)
		}
		from, to := options.From, options.To
		if from == "" {
			from = options.Root
		}
		if to == "" {
			to = options.Root
		}
		source, err := r.fileRoot(from)
		if err != nil {
			return r.raise(L, err)
		}
		defer source.Close()
		dest, err := r.fileRoot(to)
		if err != nil {
			return r.raise(L, err)
		}
		defer dest.Close()
		var result fileOutcome
		if operation == "copy" {
			result, err = r.copyFile(source, name, dest, destName, options)
		} else {
			result, err = r.symlinkFile(source, name, dest, destName, options.Replace)
		}
		if err != nil {
			return r.raise(L, err)
		}
		return r.pushValue(L, result)
	}
	root, err := r.fileRoot(options.Root)
	if err != nil {
		return r.raise(L, err)
	}
	defer root.Close()
	switch operation {
	case "path":
		L.Push(lua.LString(filepath.Join(root.Name(), name)))
		return 1
	case "read":
		data, err := r.readFile(root, name)
		if err != nil {
			return r.raise(L, err)
		}
		L.Push(lua.LString(data))
		return 1
	case "exists", "stat":
		info, err := r.statFile(root, name, options.Follow)
		if err != nil {
			return r.raise(L, err)
		}
		if operation == "exists" {
			L.Push(lua.LBool(info != nil))
			return 1
		}
		return r.pushValue(L, info)
	case "list":
		entries, err := r.listFiles(root, name)
		if err != nil {
			return r.raise(L, err)
		}
		return r.pushValue(L, entries)
	case "readlink":
		info, err := root.Lstat(name)
		if err != nil {
			return r.raise(L, fileError(err))
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return r.raise(L, Fail("invalid_file_type", "readlink requires a symbolic link"))
		}
		target, err := root.Readlink(name)
		if err != nil {
			return r.raise(L, fileError(err))
		}
		L.Push(lua.LString(target))
		return 1
	case "write":
		result, err := r.writeFile(root, name, []byte(L.CheckString(2)), options.Replace)
		if err != nil {
			return r.raise(L, err)
		}
		return r.pushValue(L, result)
	case "mkdir":
		if err := mutationPath(root, name); err != nil {
			return r.raise(L, err)
		}
		status := "created"
		if info, err := root.Lstat(name); err == nil {
			if !info.IsDir() {
				return r.raise(L, Fail("invalid_file_type", "mkdir destination is not a directory"))
			}
			status = "reused"
		} else if !os.IsNotExist(err) {
			return r.raise(L, fileError(err))
		}
		if err := root.MkdirAll(name, 0755); err != nil {
			return r.raise(L, fileError(err))
		}
		return r.pushValue(L, fileOutcome{Status: status, Path: filepath.ToSlash(name)})
	case "remove":
		if name == ".wtf" {
			return r.raise(L, Fail("protected_path", "Use setup teardown to remove the WTF configuration directory"))
		}
		if err := mutationPath(root, name); err != nil {
			return r.raise(L, err)
		}
		if _, err := root.Lstat(name); os.IsNotExist(err) {
			return r.pushValue(L, fileOutcome{Status: "missing", Path: filepath.ToSlash(name)})
		} else if err != nil {
			return r.raise(L, fileError(err))
		}
		if err := r.removalPath(root, name, options.Recursive); err != nil {
			return r.raise(L, err)
		}
		if options.Recursive {
			err = root.RemoveAll(name)
		} else {
			err = root.Remove(name)
		}
		if err != nil {
			return r.raise(L, fileError(err))
		}
		return r.pushValue(L, fileOutcome{Status: "removed", Path: filepath.ToSlash(name)})
	}
	return r.raise(L, Fail("invalid_host_api", "Unknown file operation"))
}

func (r *Runtime) commandCall(L *lua.LState, checkOnly bool) int {
	argv, err := tableStrings(L, L.Get(1))
	if err != nil {
		return r.raise(L, err)
	}
	opts, err := luaOptions(L, L.Get(2), "root", "cwd", "env", "inherit_env", "timeout_seconds", "stdin", "check")
	if err != nil {
		return r.raise(L, err)
	}
	scope, err := stringOption(opts, "root", "")
	if err != nil {
		return r.raise(L, err)
	}
	cwd, err := stringOption(opts, "cwd", ".")
	if err != nil {
		return r.raise(L, err)
	}
	name, err := fileName(cwd, false)
	if err != nil {
		return r.raise(L, err)
	}
	root, err := r.fileRoot(scope)
	if err != nil {
		return r.raise(L, err)
	}
	defer root.Close()
	f, err := r.openRead(root, name)
	if err != nil {
		return r.raise(L, err)
	}
	info, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return r.raise(L, fileError(err))
	}
	if !info.IsDir() {
		return r.raise(L, Fail("invalid_path", "Command cwd must be a directory"))
	}
	actual, err := filepath.EvalSymlinks(filepath.Join(root.Name(), name))
	if err != nil {
		return r.raise(L, fileError(err))
	}
	set, unset, err := luaEnvironment(opts.RawGetString("env"))
	if err != nil {
		return r.raise(L, err)
	}
	inherit, err := boolOption(opts, "inherit_env", true)
	if err != nil {
		return r.raise(L, err)
	}
	check, err := boolOption(opts, "check", true)
	if err != nil {
		return r.raise(L, err)
	}
	seconds, err := timeoutOption(opts)
	if err != nil {
		return r.raise(L, err)
	}
	stdin, err := stringOption(opts, "stdin", "")
	if err != nil {
		return r.raise(L, err)
	}
	result, err := r.App.RunCommand(r.Context, actual, argv, CommandOptions{Env: set, Unset: unset, NoInheritEnv: !inherit, Timeout: time.Duration(seconds) * time.Second, Stdin: stdin, Capture: true})
	if err != nil && (ErrorCode(err) == "command_cancelled" || !result.Started && !result.TimedOut) {
		return r.raise(L, err)
	}
	if checkOnly {
		L.Push(lua.LBool(result.OK))
		return 1
	}
	if err != nil && check {
		return r.raise(L, err)
	}
	return r.pushValue(L, result)
}

func (r *Runtime) environmentCall(L *lua.LState, operation string) int {
	switch operation {
	case "merge":
		result := L.NewTable()
		for i := 1; i <= L.GetTop(); i++ {
			if _, _, err := luaEnvironment(L.Get(i)); err != nil {
				return r.raise(L, err)
			}
			table, ok := L.Get(i).(*lua.LTable)
			if !ok {
				return r.raise(L, Fail("invalid_environment", "merge expects environment tables"))
			}
			table.ForEach(func(key, value lua.LValue) { result.RawSet(key, value) })
		}
		L.Push(result)
		return 1
	case "select":
		names, err := tableStrings(L, L.Get(1))
		if err != nil {
			return r.raise(L, err)
		}
		result := L.NewTable()
		for _, name := range names {
			if err := validEnv(name, ""); err != nil {
				return r.raise(L, err)
			}
			if value, exists := os.LookupEnv(name); exists {
				result.RawSetString(name, lua.LString(value))
			}
		}
		L.Push(result)
		return 1
	default:
		name := L.CheckString(1)
		if err := validEnv(name, ""); err != nil {
			return r.raise(L, err)
		}
		value, exists := os.LookupEnv(name)
		if operation == "has" {
			L.Push(lua.LBool(exists))
			return 1
		}
		if !exists && operation == "require" {
			return r.raise(L, Fail("environment_missing", "Required environment variable is absent: "+name))
		}
		if !exists {
			if L.Get(2) != lua.LNil {
				L.Push(lua.LString(L.CheckString(2)))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		}
		L.Push(lua.LString(value))
		return 1
	}
}

func (r *Runtime) installSubstrate(module *lua.LTable) error {
	L := r.L
	fs, ok := module.RawGetString("fs").(*lua.LTable)
	if !ok {
		return Fail("invalid_host_api", "Missing filesystem namespace")
	}
	for _, name := range []string{"exists", "stat", "read", "write", "copy", "symlink", "readlink", "list", "mkdir", "remove", "path"} {
		fs.RawSetString(name, L.NewFunction(func(L *lua.LState) int { return r.fileCall(L, name) }))
	}
	exec, ok := module.RawGetString("exec").(*lua.LTable)
	if !ok {
		return Fail("invalid_host_api", "Missing command namespace")
	}
	exec.RawSetString("run", L.NewFunction(func(L *lua.LState) int { return r.commandCall(L, false) }))
	exec.RawSetString("check", L.NewFunction(func(L *lua.LState) int { return r.commandCall(L, true) }))
	env := L.NewTable()
	for _, name := range []string{"get", "has", "require", "select", "merge"} {
		env.RawSetString(name, L.NewFunction(func(L *lua.LState) int { return r.environmentCall(L, name) }))
	}
	module.RawSetString("env", env)
	r.Modules["wtf.env"] = env
	return nil
}
