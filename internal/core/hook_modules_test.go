package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func packageFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, ".wtf", "hooks")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, ".wtf", "config.toml"), []byte("version=1\nlua='hooks/init.lua'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestModulesRegisterExplicitlyAndForwardRequests(t *testing.T) {
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{
		"init.lua": `local recipes=require("recipes"); assert(recipes==require("recipes"));return {hooks={after=require("after")},overrides={new=recipes.create}}`,
		"new.lua":  `error("A filename must not register or execute itself")`,
		"recipes.lua": `return {create=function(ctx,request,default)
 ctx:require(ctx.workspace==nil,"No workspace should exist before default")
 ctx:require(ctx.project.path~=nil and ctx:check({"sh","-c","test -f input.txt"}),"Source check failed")
 local w=default(request);ctx:hook("after",request);return w end}`,
		"after.lua": `return function(ctx,request)
 ctx:require(request.args[1]=="--MiXeD" and request.args[2]=="ValueCase","Arguments changed")
 ctx:stage("after",{"sh","-c","touch after.count"}) end`,
	})
	child, e := a.NewWorkspace(context.Background(), "explicit", "@", "--MiXeD", "ValueCase")
	if e != nil {
		t.Fatal(e)
	}
	if child.Stages["after"].Status != "succeeded" {
		t.Fatal(child)
	}
	names, e := a.Hooks(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(names["hooks"]) != 1 || names["hooks"][0] != "after" || len(names["overrides"]) != 1 {
		t.Fatalf("unexpected registrations %+v", names)
	}
}

func TestGuardBlocksBeforeWorkspaceMutation(t *testing.T) {
	a, r, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{
		"init.lua":    `return {overrides={new=require("guarded")}}`,
		"guarded.lua": `return function(ctx,request,default) ctx:require(ctx:check({"sh","-c","exit 3"}),"Project policy blocked creation");return default(request) end`,
	})
	before, e := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	child, e := a.NewWorkspace(context.Background(), "blocked", "@")
	if ErrorCode(e) != "hook_blocked" || child.ID != "" {
		t.Fatalf("guard failed %+v %v", child, e)
	}
	after, e := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if e != nil || string(before) != string(after) {
		t.Fatal("blocked hook mutated registry")
	}
	if _, e = os.Stat(filepath.Join(filepath.Dir(w.Path), "blocked--"+filepath.Base(w.Path))); !os.IsNotExist(e) {
		t.Fatal("blocked hook created directory")
	}
}

func TestModuleCyclesAndEscapesAreTypedFailures(t *testing.T) {
	for _, tc := range []struct {
		name, init string
		files      map[string]string
		code       string
	}{
		{"cycle", `return require("lib.a")`, map[string]string{"lib/a.lua": `return require("lib.b")`, "lib/b.lua": `return require("lib.a")`}, "module_cycle"},
		{"escape", `return require("../outside")`, nil, "invalid_module"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, w := fixture(t)
			files := map[string]string{"init.lua": tc.init}
			for name, body := range tc.files {
				files[name] = body
			}
			packageFiles(t, w.Path, files)
			_, e := a.Hooks(context.Background())
			if ErrorCode(e) != tc.code {
				t.Fatalf("expected %s got %v", tc.code, e)
			}
		})
	}
}

func TestCaughtHostErrorDoesNotReplaceLaterScriptError(t *testing.T) {
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{"init.lua": `return {overrides={new=function(ctx,request,default)
 local ok,err=pcall(function() ctx:block("first") end)
 assert(not ok and err.code=="hook_blocked" and err.message=="first")
 error("later script failure") end}}`})
	_, e := a.NewWorkspace(context.Background(), "error-context", "@")
	if ErrorCode(e) != "hook_failed" || !strings.Contains(e.Error(), "later script failure") {
		t.Fatalf("stale error leaked: %v", e)
	}
}

func TestUnconfiguredDirectoryDoesNotEnableHooks(t *testing.T) {
	a, _, w := fixture(t)
	dir := filepath.Join(w.Path, ".wtf", "hooks")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "new.lua"), []byte(`error("must not run")`), 0600); e != nil {
		t.Fatal(e)
	}
	child, e := a.NewWorkspace(context.Background(), "barebones", "@")
	if e != nil || child.Status != "ready" {
		t.Fatalf("filename enabled override: %+v %v", child, e)
	}
}

func TestManualHookReceivesArguments(t *testing.T) {
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{"init.lua": `return {hooks={inspect=function(ctx,request) ctx:require(request.args[1]=="KeepCase","Missing forwarded argument") end}}`})
	if _, e := a.Hook(context.Background(), "inspect", "KeepCase"); e != nil {
		t.Fatal(e)
	}
}
