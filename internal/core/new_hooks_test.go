package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func creationHookFiles(t *testing.T, root string, names []string, files map[string]string) {
	t.Helper()
	packageFiles(t, root, files)
	array, err := json.Marshal(names)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wtf", "config.toml"), []byte("version=1\n[new]\nhooks="+string(array)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCreationHooksRunInOrderAfterCreationWithoutEntrypoint(t *testing.T) {
	a, _, root := fixture(t)
	creationHookFiles(t, root.Path, []string{"FirstCase", "second"}, map[string]string{
		"FirstCase.lua": `local wtf = require("wtf")
return function(event)
  assert(event.workspace and event.workspace.path == event.path)
  assert(event.name == event.workspace.name and event.ctx == wtf.ctx)
  assert(event.default == nil and event.from == "@")
  assert(event.args[1] == "--KeepCase" and event.args[2] == "ValueCase")
  wtf.exec.stage("first", {"sh", "-c", "printf first >> order.count"})
end`,
		"second.lua": `local wtf = require("wtf")
return function(event)
  assert(wtf.exec.check({"sh", "-c", "test \"$(cat order.count)\" = first"}))
  wtf.exec.stage("second", {"sh", "-c", "printf second >> order.count"})
end`,
		"unlisted.lua": `error("Unlisted modules must not execute")`,
	})
	// Syntax validation compiles selected modules but invokes no Lua code.
	if _, err := LoadConfig(root.Path); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(context.Background(), "../declarative", "@", "--KeepCase", "ValueCase")
	if err != nil || child.Status != "ready" || child.Stages["first"].Status != "succeeded" || child.Stages["second"].Status != "succeeded" {
		t.Fatalf("creation hooks: %+v %v", child, err)
	}
	data, err := os.ReadFile(filepath.Join(child.Path, "order.count"))
	if err != nil || string(data) != "firstsecond" {
		t.Fatalf("incorrect hook order: %q %v", data, err)
	}
	if _, err := a.NewWorkspace(context.Background(), child.Path, ""); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(child.Path, "order.count"))
	if err != nil || string(data) != "firstsecond" {
		t.Fatalf("existing workspace reran creation hooks: %q %v", data, err)
	}
	names, err := a.Hooks(context.Background())
	if err != nil || len(names["hooks"]) != 2 || len(names["overrides"]) != 0 {
		t.Fatalf("TOML-selected hooks were not registered: %+v %v", names, err)
	}
}

func TestCreationHookFailureStopsSequenceAndRetainsWorkspace(t *testing.T) {
	a, r, root := fixture(t)
	creationHookFiles(t, root.Path, []string{"first", "block", "last"}, map[string]string{
		"first.lua": `return function(event) require("wtf.exec").stage("completed", {"sh", "-c", "touch completed.count"}) end`,
		"block.lua": `return function(event) require("wtf.policy").block("Repair this workspace") end`,
		"last.lua":  `return function(event) require("wtf.exec").stage("last", {"sh", "-c", "touch last.count"}) end`,
	})
	child, err := a.NewWorkspace(context.Background(), "../hook-failure", "@")
	if ErrorCode(err) != "hook_blocked" || child.ID == "" || child.Status != "hook_failed" || child.Stages["completed"].Status != "succeeded" {
		t.Fatalf("failure lost workspace evidence: %+v %v", child, err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, "completed.count")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, "last.count")); !os.IsNotExist(err) {
		t.Fatalf("sequence continued after failure: %v", err)
	}
	state, err := r.Load()
	if err != nil || state.Workspaces[child.ID].Status != "hook_failed" {
		t.Fatalf("failed workspace not retained: %+v %v", state, err)
	}
}

func TestCreationHookInvalidConfigurationBlocksBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name, config, code string
		files              map[string]string
	}{
		{"missing", "version=1\n[new]\nhooks=['missing']\n", "hook_missing", nil},
		{"syntax", "version=1\n[new]\nhooks=['bad']\n", "invalid_hook", map[string]string{"bad.lua": `return function(`}},
		{"type", "version=1\n[new]\nhooks=['bad']\n", "invalid_hook", map[string]string{"bad.lua": `return {}`}},
		{"escape", "version=1\n[new]\nhooks=['../outside']\n", "invalid_config", nil},
		{"reserved", "version=1\n[new]\nhooks=['wtf.exec']\n", "invalid_config", nil},
		{"mixed", "version=1\nlua='hooks/init.lua'\n[new]\nhooks=['hello']\n", "invalid_config", nil},
		{"wrong type", "version=1\n[new]\nhooks=[1]\n", "invalid_config", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r, root := fixture(t)
			packageFiles(t, root.Path, tc.files)
			if err := os.WriteFile(filepath.Join(root.Path, ".wtf", "config.toml"), []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			child, err := a.NewWorkspace(context.Background(), "../invalid-hook", "@")
			if ErrorCode(err) != tc.code || child.ID != "" {
				t.Fatalf("invalid configuration: %+v %v", child, err)
			}
			after, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
			if err != nil || string(before) != string(after) {
				t.Fatalf("configuration failure mutated registry: %v", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(root.Path), "invalid-hook")); !os.IsNotExist(err) {
				t.Fatalf("configuration failure created workspace: %v", err)
			}
		})
	}
}

func TestCreationHookValidationDoesNotExecuteModules(t *testing.T) {
	_, _, root := fixture(t)
	creationHookFiles(t, root.Path, []string{"hello"}, map[string]string{"hello.lua": `error("Syntax checks must not execute this module")`})
	if _, err := LoadConfig(root.Path); err != nil {
		t.Fatalf("validation executed module: %v", err)
	}
}
