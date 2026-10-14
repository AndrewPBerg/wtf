package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLuaPackageImportsCallbacksAndStageReuse(t *testing.T) {
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{
		"wtf.lua": `error("Project code must not shadow the host package")`,
		"init.lua": `local wtf = require("wtf")
assert(wtf == require("wtf") and wtf.api_version == 1)
assert(wtf.fs == require("wtf.fs"))
assert(wtf.exec == require("wtf.exec"))
assert(wtf.policy == require("wtf.policy"))
assert(wtf.resources == require("wtf.resources"))
assert(wtf.hooks == require("wtf.hooks"))
return wtf.setup {
  hooks = {
    prepare = require("prepare"),
    inspect = function(event)
      assert(event.workspace.id == wtf.ctx.workspace.id)
      assert(event.ctx == wtf.ctx and event.project.path == wtf.ctx.project.path)
      assert(event.name == event.request.name and event.from == "")
      assert(event.args[1] == "--MiXeD" and event.args[2] == "ValueCase")
      assert(event.default == nil)
      assert(wtf.hooks.has("prepare") and not wtf.hooks.has("absent"))
      assert(wtf.exec.check({"sh", "-c", "exit 0"}))
      assert(not wtf.exec.check({"sh", "-c", "exit 4"}))
      assert(wtf.fs.exists("input.txt") and not wtf.fs.exists("absent.txt"))
      assert(type(wtf.resources.listening(65535)) == "boolean")
      wtf.policy.require(true, "allowed")
      local ok, err = pcall(function() wtf.policy.block("blocked") end)
      assert(not ok and err.code == "hook_blocked" and err.message == "blocked")
      local claimed = wtf.resources.claim("recipe", "sandbox", "ExampleCase")
      assert(claimed == "ExampleCase")
      local claims = wtf.resources.list()
      assert(#claims == 1 and claims[1].name == "sandbox" and claims[1].kind == "recipe")
      wtf.resources.release("recipe", "sandbox")
      assert(#wtf.resources.list() == 0)
      local ok, err = pcall(function() wtf.resources.claim_port("bad", 0) end)
      assert(not ok and err.code == "invalid_port")
      wtf.hooks.run("prepare")
    end,
  },
}`,
		"prepare.lua": `local exec = require("wtf.exec")
return function(event)
  assert(event.args[1] == "--MiXeD")
  local stage = exec.stage("package-prepare", {"sh", "-c", "printf x >> package.count"}, {
    inputs = {"input.txt"}, reuse = true, timeout_seconds = 10,
    env = {WTF_PACKAGE_TEST = "ValueCase"},
    check = {"sh", "-c", "test \"$WTF_PACKAGE_TEST\" = ValueCase"},
  })
  assert(stage.status == "succeeded" and type(stage.reused) == "boolean")
end`,
	})
	for range 2 {
		if _, err := a.Hook(context.Background(), "inspect", "--MiXeD", "ValueCase"); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(w.Path, "package.count"))
	if err != nil || string(data) != "x" {
		t.Fatalf("package stage was not reused: %q %v", data, err)
	}
}

func TestLuaPackageLifecycleDefaultUpdatesEventAndPreservesIdentity(t *testing.T) {
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{"init.lua": `local wtf = require("wtf")
return wtf.setup {
  hooks = {prepare = function(event)
    assert(event.workspace and event.args[1] == "KeepCase")
    wtf.exec.stage("prepared", {"sh", "-c", "touch package.count"})
  end},
  overrides = {
    new = function(event)
      assert(event.workspace == nil and event.path and event.default)
      wtf.policy.require(wtf.fs.exists("input.txt"), "missing source")
      local workspace = event.default()
      assert(workspace.id == event.workspace.id)
      return workspace
    end,
    init = function(event)
      local workspace = event.default()
      assert(workspace.id == event.workspace.id)
      wtf.hooks.run("prepare")
      return workspace
    end,
    remove = function(event)
      assert(event.workspace and event.default)
      return event.default()
    end,
  },
}`})
	child, err := a.NewWorkspace(context.Background(), "../package-child", "@", "KeepCase")
	if err != nil || child.Stages["prepared"].Status != "succeeded" {
		t.Fatalf("package lifecycle: %+v %v", child, err)
	}
	// Identity checks still reject a replacement request targeting another workspace.
	packageFiles(t, w.Path, map[string]string{"init.lua": `return require("wtf").setup {
overrides = {remove = function(event)
  return event.default({name = "wrong", from = "", args = {}})
end}}`})
	// Removal reads the target's config, so apply the same guarded entrypoint there.
	packageFiles(t, child.Path, map[string]string{"init.lua": `return require("wtf").setup {
overrides = {remove = function(event)
  return event.default({name = "wrong", from = "", args = {}})
end}}`})
	_, err = a.RemoveWithOptions(context.Background(), child.Name, RemovalOptions{Force: true})
	if ErrorCode(err) != "identity_mismatch" {
		t.Fatalf("package bypassed lifecycle identity: %v", err)
	}
}

func TestLuaPackageRejectsInvalidSetupAndReservedModules(t *testing.T) {
	for _, source := range []string{
		`return require("wtf").setup(false)`,
		`return require("wtf").setup {typo = {}}`,
		`return require("wtf").setup {[""] = {}}`,
		`return require("wtf").setup {hooks = {prepare = "not a function"}}`,
		`return require("wtf").setup {overrides = {typo = function() end}}`,
	} {
		t.Run(source, func(t *testing.T) {
			a, _, w := fixture(t)
			packageFiles(t, w.Path, map[string]string{"init.lua": source})
			if _, err := a.Hooks(context.Background()); ErrorCode(err) != "invalid_hook" {
				t.Fatalf("invalid setup contract: %v", err)
			}
		})
	}
	a, _, w := fixture(t)
	packageFiles(t, w.Path, map[string]string{
		"init.lua":        `return require("wtf.unknown")`,
		"wtf/unknown.lua": `return {}`,
	})
	if _, err := a.Hooks(context.Background()); ErrorCode(err) != "invalid_module" {
		t.Fatalf("project shadowed a reserved namespace: %v", err)
	}
}
