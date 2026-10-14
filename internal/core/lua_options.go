package core

import (
	"math"

	lua "github.com/yuin/gopher-lua"
)

func luaOptions(state *lua.LState, value lua.LValue, keys ...string) (*lua.LTable, error) {
	if value == lua.LNil {
		return state.NewTable(), nil
	}
	options, ok := value.(*lua.LTable)
	if !ok {
		return nil, Fail("invalid_options", "Expected an options table")
	}
	allowed := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	var bad string
	var invalid bool
	options.ForEach(func(key, _ lua.LValue) {
		if key.Type() != lua.LTString || !allowed[key.String()] {
			bad = key.String()
			invalid = true
		}
	})
	if invalid {
		return nil, Fail("invalid_options", "Unknown option: "+bad)
	}
	return options, nil
}

func stringOption(options *lua.LTable, key, fallback string) (string, error) {
	value := options.RawGetString(key)
	if value == lua.LNil {
		return fallback, nil
	}
	str, ok := value.(lua.LString)
	if !ok {
		return "", Fail("invalid_options", key+" must be a string")
	}
	return string(str), nil
}
func boolOption(options *lua.LTable, key string, fallback bool) (bool, error) {
	value := options.RawGetString(key)
	if value == lua.LNil {
		return fallback, nil
	}
	b, ok := value.(lua.LBool)
	if !ok {
		return false, Fail("invalid_options", key+" must be a boolean")
	}
	return bool(b), nil
}
func timeoutOption(options *lua.LTable) (int, error) {
	value := options.RawGetString("timeout_seconds")
	if value == lua.LNil {
		return 120, nil
	}
	n, ok := value.(lua.LNumber)
	if !ok || math.Trunc(float64(n)) != float64(n) || n < 1 || n > 3600 {
		return 0, Fail("invalid_timeout", "timeout_seconds must be an integer from 1 to 3600")
	}
	return int(n), nil
}

func luaEnvironment(value lua.LValue) (map[string]string, []string, error) {
	set := map[string]string{}
	var unset []string
	if value == lua.LNil {
		return set, unset, nil
	}
	table, ok := value.(*lua.LTable)
	if !ok {
		return nil, nil, Fail("invalid_environment", "env must be a map of string values or false to unset")
	}
	var failure error
	table.ForEach(func(key, value lua.LValue) {
		if failure != nil {
			return
		}
		name, ok := key.(lua.LString)
		if !ok {
			failure = Fail("invalid_environment", "Environment keys must be strings")
			return
		}
		if value == lua.LFalse {
			failure = validEnv(string(name), "")
			unset = append(unset, string(name))
			return
		}
		text, ok := value.(lua.LString)
		if !ok {
			failure = Fail("invalid_environment", "Environment values must be strings or false")
			return
		}
		failure = validEnv(string(name), string(text))
		set[string(name)] = string(text)
	})
	return set, unset, failure
}
