package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CommandDefaults loads inherited declarative defaults without executing hooks.
func (a *App) CommandDefaults(ctx context.Context, canonical func(string) (string, error)) (map[string]map[string]any, error) {
	if ctx.Err() != nil {
		return nil, Fail("command_cancelled", ctx.Err().Error())
	}
	leaf, e := filepath.Abs(a.Cwd)
	if e != nil {
		return nil, e
	}
	parents := []string{}
	for dir := leaf; ; dir = filepath.Dir(dir) {
		parents = append(parents, filepath.Join(dir, ".wtf", "config.toml"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	paths := []string{filepath.Join(a.Home, "config.toml")}
	for i := len(parents) - 1; i >= 0; i-- {
		paths = append(paths, parents[i])
	}
	result := map[string]map[string]any{}
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		var cfg Config
		if e = toml.NewDecoder(strings.NewReader(string(b))).DisallowUnknownFields().Decode(&cfg); e != nil {
			return nil, Fail("invalid_config", path+": "+e.Error())
		}
		if cfg.Version != 1 {
			return nil, Fail("invalid_config", path+": expected version = 1")
		}
		seen := map[string]bool{}
		for command, flags := range cfg.Commands {
			key := strings.ToLower(strings.Join(strings.Fields(command), " "))
			if canonical != nil {
				key, e = canonical(key)
				if e != nil {
					return nil, e
				}
			}
			if seen[key] {
				return nil, Fail("invalid_config", path+": duplicate command sections through aliases")
			}
			seen[key] = true
			if result[key] == nil {
				result[key] = map[string]any{}
			}
			for flag, value := range flags {
				switch value.(type) {
				case bool, string, int64:
				default:
					return nil, Fail("invalid_config", path+": command flag defaults must be booleans, strings or integers")
				}
				result[key][strings.ToLower(flag)] = value
			}
		}
	}
	return result, nil
}
