package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaFilesystemCompleteSurfaceAndCrossWorkspaceLinks(t *testing.T) {
	a, _, source := fixture(t)
	creationHookFiles(t, source.Path, []string{"files"}, map[string]string{"files.lua": `local fs=require("wtf.fs")
local function fault(code, fn)
  local ok, err=pcall(fn); assert(not ok and err.code==code, tostring(err))
end
return function(event)
  assert(event.project.path ~= event.workspace.path)
  assert(fs.mkdir(".wtf/local/files").status=="created")
  assert(fs.mkdir(".wtf/local/files").status=="reused")
  local dir=".wtf/local/files/"
  assert(fs.write(dir.."note", "Hello\0World").status=="written")
  assert(fs.read(dir.."note")=="Hello\0World")
  assert(fs.write(dir.."note", "ignored").status=="preserved")
  assert(fs.read(dir.."note")=="Hello\0World")
  assert(fs.write(dir.."note", "replaced", {replace=true}).status=="written")
  assert(fs.copy("input.txt",dir.."source-copy",{from="project",to="workspace"}).status=="copied")
  assert(fs.read(dir.."source-copy")=="first")
  assert(fs.copy("input.txt",dir.."source-copy",{from="project",to="workspace"}).status=="preserved")
  assert(fs.symlink("input.txt",dir.."source-link",{from="project",to="workspace"}).status=="linked")
  assert(fs.symlink("input.txt",dir.."source-link",{from="project",to="workspace"}).status=="reused")
  assert(fs.readlink(dir.."source-link")==fs.path("input.txt",{root="project"}))
  assert(fs.exists(dir.."source-link"))
  assert(fs.stat(dir.."source-link").type=="symlink")
  assert(fs.stat(dir.."source-link",{follow=true}).type=="file")
  assert(fs.read(dir.."source-link")=="first")
  fault("invalid_options",function() fs.copy(dir.."source-link",dir.."ambiguous") end)
  assert(not fs.exists(dir.."ambiguous"))
  assert(fs.copy(dir.."source-link",dir.."preserved-link",{symlinks="preserve"}).status=="copied")
  assert(fs.readlink(dir.."preserved-link")==fs.readlink(dir.."source-link"))
  assert(fs.copy(dir.."source-link",dir.."followed",{symlinks="follow"}).status=="copied")
  assert(fs.stat(dir.."followed").type=="file" and fs.read(dir.."followed")=="first")
  fs.write(dir.."source-link","independent",{replace=true})
  assert(fs.stat(dir.."source-link").type=="file")
  assert(fs.read("input.txt",{root="project"})=="first")
  fs.symlink(dir.."missing",dir.."broken")
  assert(fs.exists(dir.."broken") and not fs.exists(dir.."broken",{follow=true}))
  assert(fs.stat(dir.."broken").type=="symlink")
  assert(fs.remove(dir.."broken").status=="removed")
  assert(fs.remove(dir.."broken").status=="missing")
  fs.mkdir(dir.."tree/sub")
  fs.write(dir.."tree/sub/data","nested")
  fs.symlink(dir.."tree",dir.."directory-link")
  assert(fs.remove(dir.."directory-link",{recursive=true}).status=="removed")
  assert(fs.read(dir.."tree/sub/data")=="nested")
  assert(fs.copy(dir.."tree",dir.."tree-copy",{recursive=true}).status=="copied")
  assert(fs.read(dir.."tree-copy/sub/data")=="nested")
  fault("invalid_path",function() fs.copy(dir.."tree",dir.."tree/child",{recursive=true}) end)
  fs.symlink(dir.."tree",dir.."tree-alias")
  fault("invalid_path",function() fs.copy(dir.."tree-alias",dir.."tree/child",{recursive=true,symlinks="follow"}) end)
  local ok=pcall(function() fs.remove(dir.."tree") end); assert(not ok)
  fs.remove(dir.."tree-copy",{recursive=true})
  assert(not fs.exists(dir.."tree-copy"))
  assert(fs.stat(dir.."absent")==nil)
  fault("file_missing",function() fs.read(dir.."absent") end)
  fault("invalid_file_type",function() fs.readlink(dir.."note") end)
  fault("invalid_path",function() fs.read("../escape") end)
  fault("invalid_path",function() fs.write("/tmp/escape","x") end)
  fault("protected_path",function() fs.write(".wtf/workspace.json","x",{replace=true}) end)
  fault("protected_path",function() fs.remove(".jj",{recursive=true}) end)
  fault("protected_path",function() fs.remove(".",{recursive=true}) end)
  fault("protected_path",function() fs.remove(".wtf",{recursive=true}) end)
  fault("invalid_options",function() fs.write(dir.."bad","x",{replcae=true}) end)
  local entries=fs.list(dir)
  for i=2,#entries do assert(entries[i-1].name<entries[i].name) end
  for _,entry in ipairs(entries) do assert(not string.find(entry.name,"^%.wtf%-")) end
end`})
	child, err := a.NewWorkspace(t.Context(), "../files-child", "@")
	if err != nil {
		t.Fatal(err)
	}
	if child.Status != "ready" {
		t.Fatalf("files hook did not finish: %+v", child)
	}
}

func TestLuaFilesystemEscapesCyclesAndAtomicFailures(t *testing.T) {
	a, _, w := fixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(w.Path, "external")); err != nil {
		t.Fatal(err)
	}
	creationHookFiles(t, w.Path, []string{"security"}, map[string]string{"security.lua": `local fs=require("wtf.fs")
local function blocked(fn) local ok,err=pcall(fn); assert(not ok and err.code) end
return function()
  blocked(function() fs.read("external/keep") end)
  blocked(function() fs.write("external/keep","changed",{replace=true}) end)
  fs.mkdir(".wtf/local/security/tree")
  fs.write(".wtf/local/security/tree/regular","safe")
  fs.symlink(".wtf/local/security/tree",".wtf/local/security/tree/cycle")
  blocked(function() fs.copy(".wtf/local/security/tree",".wtf/local/security/copy",{recursive=true,symlinks="follow"}) end)
  assert(not fs.exists(".wtf/local/security/copy"))
  fs.mkdir(".wtf/local/security/destination")
  fs.write(".wtf/local/security/destination/keep","user edits")
  blocked(function() fs.copy(".wtf/local/security/tree",".wtf/local/security/destination",{recursive=true,symlinks="preserve",replace=true}) end)
  assert(fs.read(".wtf/local/security/destination/keep")=="user edits")
  for _,entry in ipairs(fs.list(".wtf/local/security")) do assert(not string.find(entry.name,"^%.wtf%-")) end
end`})
	if _, err := a.Hook(t.Context(), "security"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outside, "keep"))
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside target was modified: %q %v", data, err)
	}
}

func TestLuaCommandsEnvironmentAndStageDirectories(t *testing.T) {
	a, _, w := fixture(t)
	t.Setenv("WTF_TEST_PARENT", "KeepCase")
	t.Setenv("WTF_TEST_EMPTY", "")
	creationHookFiles(t, w.Path, []string{"commands"}, map[string]string{"commands.lua": `local wtf=require("wtf")
local env=require("wtf.env")
local exec=require("wtf.exec")
local fs=require("wtf.fs")
local function fault(code, fn) local ok,err=pcall(fn); assert(not ok and err.code==code,tostring(err)) end
return function()
  assert(env.has("WTF_TEST_PARENT") and env.get("WTF_TEST_PARENT")=="KeepCase")
  assert(env.has("WTF_TEST_EMPTY") and env.get("WTF_TEST_EMPTY")=="")
  assert(env.get("WTF_TEST_ABSENT")==nil and env.get("WTF_TEST_ABSENT","fallback")=="fallback")
  assert(env.require("WTF_TEST_PARENT")=="KeepCase")
  fault("environment_missing",function() env.require("WTF_TEST_ABSENT") end)
  local selected=env.select({"WTF_TEST_PARENT","WTF_TEST_ABSENT"})
  assert(selected.WTF_TEST_PARENT=="KeepCase" and selected.WTF_TEST_ABSENT==nil)
  local merged=env.merge(selected,{WTF_TEST_PARENT=false,EXPLICIT="MiXeD",EMPTY=""})
  assert(selected.WTF_TEST_PARENT=="KeepCase" and merged.WTF_TEST_PARENT==false)
  fs.mkdir(".wtf/local/commands")
  local result=exec.run({"sh","-c",'printf "%s|%s|%s" "$EXPLICIT" "${WTF_TEST_PARENT-unset}" "$EMPTY"; printf error >&2; cat'}, {
    cwd=".wtf/local/commands",env=merged,stdin="input",timeout_seconds=5,
  })
  assert(result.ok and result.started and result.code==0)
  assert(result.stdout=="MiXeD|unset|input" and result.stderr=="error")
  assert(env.get("WTF_TEST_PARENT")=="KeepCase")
  local clean=exec.run({"sh","-c",'printf "%s|%s" "${WTF_TEST_PARENT-unset}" "$ONLY"'}, {inherit_env=false,env={ONLY="isolated"}})
  assert(clean.stdout=="unset|isolated")
  local failed=exec.run({"sh","-c","printf failure; exit 7"},{check=false})
  assert(not failed.ok and failed.code==7 and failed.stdout=="failure")
  fault("command_failed",function() exec.run({"sh","-c","exit 7"}) end)
  fault("command_failed",function() exec.run({"wtf_nonexistent_test_binary"},{check=false}) end)
  assert(not exec.check({"sh","-c","exit 8"}))
  assert(exec.check({"sh","-c",'test "$VALUE" = checked'},{cwd=".wtf/local/commands",env={VALUE="checked"}}))
  fault("invalid_environment",function() exec.run({"sh","-c","true"},{env={BAD=true}}) end)
  fault("invalid_environment",function() env.merge({["BAD=KEY"]="bad"}) end)
  fault("invalid_environment",function() env.get("BAD\0KEY") end)
  fault("invalid_path",function() exec.run({"sh","-c","true"},{cwd="../escape"}) end)
  fault("invalid_timeout",function() exec.run({"sh","-c","true"},{timeout_seconds=0}) end)
  fault("invalid_options",function() exec.run({"sh","-c","true"},{timeout=1}) end)
  local literal=exec.run({"printf","%s","$(touch unsafe)"},{cwd=".wtf/local/commands"})
  assert(literal.stdout=="$(touch unsafe)" and not fs.exists(".wtf/local/commands/unsafe"))
  local stage=exec.stage("component",{"sh","-c",'printf "%s" "$VALUE" > result'}, {
    cwd=".wtf/local/commands",env={VALUE="stage",WTF_TEST_PARENT=false},inherit_env=false,
    inputs={"input.txt"},reuse=true,check={"sh","-c",'test "$(cat result)" = stage'},
  })
  assert(stage.status=="succeeded")
  assert(fs.read(".wtf/local/commands/result")=="stage")
end`})
	for range 2 {
		if _, err := a.Hook(t.Context(), "commands"); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(w.Path, ".wtf", "local", "commands", "result"))
	if err != nil || string(data) != "stage" {
		t.Fatalf("component stage result: %q %v", data, err)
	}
}

func TestLuaCommandLimitsAndTimeout(t *testing.T) {
	a, _, w := fixture(t)
	creationHookFiles(t, w.Path, []string{"limits"}, map[string]string{"limits.lua": `local exec=require("wtf.exec")
return function()
  local large=exec.run({"sh","-c","head -c 1100000 /dev/zero"})
  assert(large.stdout_truncated and #large.stdout==1048576)
  local stderr=exec.run({"sh","-c","head -c 1100000 /dev/zero >&2"})
  assert(stderr.stderr_truncated and #stderr.stderr==1048576)
  local timeout=exec.run({"sh","-c","sleep 10"},{timeout_seconds=1,check=false})
  assert(not timeout.ok and timeout.timed_out and not timeout.cancelled)
  local ok,err=pcall(function() exec.run({"sh","-c","sleep 10"},{timeout_seconds=1}) end)
  assert(not ok and err.code=="command_timeout")
end`})
	if _, err := a.Hook(t.Context(), "limits"); err != nil {
		t.Fatal(err)
	}
}

func TestSubstrateCancelledFileAndCommandOperations(t *testing.T) {
	a, r, w := fixture(t)
	runtime, err := a.runtime(t.Context(), r, w)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime.Context = ctx
	root, err := runtime.fileRoot("workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, err = runtime.writeFile(root, "cancelled", []byte("never committed"), true)
	if err == nil {
		t.Fatal("cancelled write succeeded")
	}
	if _, err := os.Stat(filepath.Join(w.Path, "cancelled")); !os.IsNotExist(err) {
		t.Fatalf("cancelled write committed: %v", err)
	}
	_, err = a.RunCommand(ctx, w.Path, []string{"sh", "-c", "sleep 10"}, CommandOptions{Capture: true})
	if ErrorCode(err) != "command_cancelled" {
		t.Fatalf("command cancellation: %v", err)
	}
}

func TestLuaFileReadWriteLimits(t *testing.T) {
	a, _, w := fixture(t)
	if err := os.WriteFile(filepath.Join(w.Path, "large"), make([]byte, fileByteLimit+1), 0600); err != nil {
		t.Fatal(err)
	}
	creationHookFiles(t, w.Path, []string{"limits"}, map[string]string{"limits.lua": `local fs=require("wtf.fs")
return function()
  local ok,err=pcall(function() fs.read("large") end);assert(not ok and err.code=="file_limit")
  ok,err=pcall(function() fs.write(".wtf/local/oversized",string.rep("x",8388609)) end)
  assert(not ok and err.code=="file_limit" and not fs.exists(".wtf/local/oversized"))
end`})
	if _, err := a.Hook(t.Context(), "limits"); err != nil {
		t.Fatal(err)
	}
}

func TestCommandEnvironmentDoesNotLeakToStageRecords(t *testing.T) {
	a, r, w := fixture(t)
	if err := os.Mkdir(filepath.Join(w.Path, "component"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := a.Stage(t.Context(), r, w, "isolated", []string{"sh", "-c", "true"}, StageOptions{Cwd: "component", Env: map[string]string{"DEMO": "fixture-value-not-a-secret"}, EnvUnset: []string{"OTHER"}, NoInheritEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "fixture-value-not-a-secret") || strings.Contains(string(state), "OTHER") {
		t.Fatal("environment settings leaked to host records")
	}
}

func TestLuaFilesystemDoesNotDeleteNestedManagedWorkspace(t *testing.T) {
	a, _, root := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "./nested", "@")
	if err != nil {
		t.Fatal(err)
	}
	creationHookFiles(t, root.Path, []string{"protect"}, map[string]string{"protect.lua": `return function()
  local ok,err=pcall(function() require("wtf.fs").remove("nested",{recursive=true}) end)
  assert(not ok and err.code=="protected_path", "guard result: "..tostring(ok).." "..tostring(err))
end`})
	if _, err := a.Hook(t.Context(), "protect"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, ".jj")); err != nil {
		t.Fatalf("nested workspace removed: %v", err)
	}
}
