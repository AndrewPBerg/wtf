package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parentPhaseFiles(t *testing.T, root, config string, files map[string]string) {
	t.Helper()
	packageFiles(t, root, files)
	if err := os.WriteFile(filepath.Join(root, ".wtf", "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestParentHooksComposeAndIsolateImports(t *testing.T) {
	a, _, root := fixture(t)
	parent := filepath.Dir(root.Path)
	parentPhaseFiles(t, parent, "version=1\n[new]\nafter=['shared']\n", map[string]string{
		"shared.lua": `local helper=require("helper"); return function(event)
 assert(event.project.path ~= event.workspace.path)
 assert(require("helper") == helper)
 require("wtf.fs").write("parent.count", helper)
end`,
		"helper.lua": `return "parent"`,
	})
	parentPhaseFiles(t, root.Path, "version=1\n[new]\nafter=['shared']\n", map[string]string{
		"shared.lua": `local helper=require("helper"); return function(event)
 assert(require("wtf.fs").read("parent.count") == "parent")
 require("wtf.fs").write("leaf.count", helper)
end`,
		"helper.lua": `return "leaf"`,
	})
	var child Initialization
	invocation := HookInvocation{Command: "new", Args: []string{"../child"}, Path: filepath.Join(parent, "child"), From: "@"}
	_, err := a.InvokeHooks(t.Context(), invocation, func() (any, error) {
		var err error
		child, err = a.NewWorkspace(t.Context(), "../child", "@")
		return child, err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(child.Path, "leaf.count"))
	if err != nil || string(data) != "leaf" {
		t.Fatalf("leaf hook: %q %v", data, err)
	}
	childApp := *a
	childApp.Cwd = child.Path
	hooks, err := childApp.Hooks(t.Context())
	if err != nil || strings.Join(hooks["hooks"], ",") != "parent1.shared,shared" {
		t.Fatalf("inherited inspection: %v %v", hooks, err)
	}
	// Existing workspace preparation does not run creation phases again.
	if err := os.WriteFile(filepath.Join(child.Path, "parent.count"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	invocation.Path = child.Path
	_, err = a.InvokeHooks(t.Context(), invocation, func() (any, error) { return a.NewWorkspace(t.Context(), child.Path, "") })
	if err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(child.Path, "parent.count"))
	if string(data) != "edited" {
		t.Fatal("creation phases repeated")
	}
}

func TestParentCreationShorthandCopiesFromSelectedRepository(t *testing.T) {
	a, _, root := fixture(t)
	parentPhaseFiles(t, filepath.Dir(root.Path), "version=1\n[new]\nhooks=['copy']\n", map[string]string{
		"copy.lua": `return function(event)
 local fs=require("wtf.fs")
 fs.copy("input.txt", "copied.count", {from="project",to="workspace"})
end`,
	})
	child, err := a.NewWorkspace(t.Context(), "../child", "@")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(child.Path, "copied.count"))
	if err != nil || string(data) != "first" {
		t.Fatalf("copy source: %q %v", data, err)
	}
}

func TestInvalidParentHookBlocksBeforeMutation(t *testing.T) {
	a, _, root := fixture(t)
	parentPhaseFiles(t, filepath.Dir(root.Path), "version=1\n[new]\nbefore=['missing']\n", nil)
	_, err := a.InvokeHooks(t.Context(), HookInvocation{Command: "new", Path: filepath.Join(filepath.Dir(root.Path), "child")}, func() (any, error) { t.Fatal("operation reached"); return nil, nil })
	if ErrorCode(err) != "hook_missing" {
		t.Fatalf("invalid parent: %v", err)
	}
}

func TestParentHooksCoexistWithLocalLuaOverride(t *testing.T) {
	a, _, root := fixture(t)
	parentPhaseFiles(t, filepath.Dir(root.Path), "version=1\n[new]\nhooks=['shared']\n", map[string]string{"shared.lua": `return function(event) require("wtf.fs").write("parent.count","parent") end`})
	packageFiles(t, root.Path, map[string]string{"init.lua": `return require("wtf").setup { overrides={new=function(event) return event.default() end} }`})
	child, err := a.NewWorkspace(t.Context(), "../child", "@")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, "parent.count")); err != nil {
		t.Fatal(err)
	}
}

func TestParentFailureRunsInheritedCleanupWithErrorSnapshot(t *testing.T) {
	a, _, root := fixture(t)
	parentPhaseFiles(t, filepath.Dir(root.Path), "version=1\n[new]\nbefore=['block']\nfailed=['cleanup']\nfinally=['finish']\n", map[string]string{
		"block.lua": `return function(event) require("wtf.policy").block("parent blocked") end`,
		"cleanup.lua": `return function(event)
 assert(event.error.code == "hook_blocked" and event.result == nil)
 require("wtf.fs").write("cleanup.count", event.error.message)
end`,
		"finish.lua": `return function(event)
 assert(event.error.code == "hook_blocked")
 require("wtf.fs").write("finish.count", "finished")
end`,
	})
	_, err := a.InvokeHooks(t.Context(), HookInvocation{Command: "new", Path: filepath.Join(filepath.Dir(root.Path), "child")}, func() (any, error) { t.Fatal("operation reached"); return nil, nil })
	if ErrorCode(err) != "hook_blocked" {
		t.Fatalf("parent error lost: %v", err)
	}
	for _, name := range []string{"cleanup.count", "finish.count"} {
		if _, err := os.Stat(filepath.Join(root.Path, name)); err != nil {
			t.Fatal(err)
		}
	}
}
