package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewInheritsConfigurationBeforeHooksAndFromInvokingWorkspace(t *testing.T) {
	a, _, source := fixture(t)
	if _, err := a.Setup(t.Context(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	creationHookFiles(t, source.Path, []string{"verify"}, map[string]string{
		"verify.lua": `return function(event)
  local fs=require("wtf.fs")
  assert(fs.exists(".wtf/config.toml"))
  assert(fs.exists(".wtf/AGENTS.md"))
  assert(fs.read(".wtf/schema.json")=="{}\n")
  assert(fs.exists(".wtf/hooks/lib/helper.lua"))
  assert(fs.exists(".wtf/tasks/nested/example.lua"))
  assert(not fs.exists(".wtf/local/private.txt"))
  fs.write(".wtf/local/hook-ran", "yes")
end`,
		"lib/helper.lua": "return 'source'\n",
	})
	write := func(root, path, data string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(source.Path, ".wtf/tasks/nested/example.lua", "return 'source task'\n")
	write(source.Path, ".wtf/local/private.txt", "not inherited")
	write(source.Path, ".wtf/other.txt", "not inherited")
	child, err := a.NewWorkspace(t.Context(), "../inherit-child", "@")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.toml", "schema.json", ".gitignore", guideChecksumFile, "hooks/verify.lua", "hooks/lib/helper.lua", "tasks/nested/example.lua"} {
		original, err := os.ReadFile(filepath.Join(source.Path, ".wtf", name))
		if err != nil {
			t.Fatal(err)
		}
		inherited, err := os.ReadFile(filepath.Join(child.Path, ".wtf", name))
		if err != nil || string(inherited) != string(original) {
			t.Fatalf("inherit %s: %v", name, err)
		}
		info, err := os.Lstat(filepath.Join(child.Path, ".wtf", name))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("not an independent file: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(child.Path, ".wtf/other.txt")); !os.IsNotExist(err) {
		t.Fatal("unselected artifact inherited")
	}
	guide, err := os.ReadFile(filepath.Join(child.Path, ".wtf/AGENTS.md"))
	if err != nil || string(guide) != string(workspaceGuide) {
		t.Fatalf("agent guide was not inherited: %v", err)
	}
	write(child.Path, ".wtf/AGENTS.md", "# Child agent instructions\n")
	write(child.Path, ".wtf/tasks/nested/example.lua", "return 'child task'\n")
	write(child.Path, ".wtf/hooks/lib/helper.lua", "return 'child'\n")
	if _, err := a.NewWorkspace(t.Context(), child.Path, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(child.Path, ".wtf/tasks/nested/example.lua"))
	if err != nil || string(data) != "return 'child task'\n" {
		t.Fatal("preparation overwrote child configuration")
	}
	guide, err = os.ReadFile(filepath.Join(child.Path, ".wtf/AGENTS.md"))
	if err != nil || string(guide) != "# Child agent instructions\n" {
		t.Fatal("preparation overwrote child guide")
	}
	a.Cwd = filepath.Join(child.Path, ".wtf/tasks")
	grandchild, err := a.NewWorkspace(t.Context(), "inherit-grandchild", "@")
	if err != nil {
		t.Fatal(err)
	}
	guide, err = os.ReadFile(filepath.Join(grandchild.Path, ".wtf/AGENTS.md"))
	if err != nil || string(guide) != "# Child agent instructions\n" {
		t.Fatalf("did not inherit invoking workspace guide: %v", err)
	}
	for _, name := range []string{"tasks/nested/example.lua", "hooks/lib/helper.lua"} {
		inherited, err := os.ReadFile(filepath.Join(grandchild.Path, ".wtf", name))
		if err != nil {
			t.Fatal(err)
		}
		custom, err := os.ReadFile(filepath.Join(child.Path, ".wtf", name))
		if err != nil || string(inherited) != string(custom) {
			t.Fatalf("did not inherit invoking workspace: %s", name)
		}
	}
	data, err = os.ReadFile(filepath.Join(source.Path, ".wtf/tasks/nested/example.lua"))
	if err != nil || string(data) != "return 'source task'\n" {
		t.Fatal("child edits changed parent")
	}
}

func TestConfigurationInheritanceMissingTreesAndUnsafeDestination(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(source, ".wtf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := inheritWorkspaceConfig(t.Context(), source, dest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hooks", "tasks"} {
		info, err := os.Stat(filepath.Join(dest, ".wtf", name))
		if err != nil || !info.IsDir() {
			t.Fatalf("empty %s missing", name)
		}
	}
	outside := t.TempDir()
	unsafe := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(unsafe, ".wtf")); err != nil {
		t.Fatal(err)
	}
	if err := inheritWorkspaceConfig(t.Context(), source, unsafe); err == nil {
		t.Fatal("followed destination .wtf symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("changed outside destination")
	}
}

func TestNewConfigurationTransferFailureRetainsWorkspaceAndSkipsHooks(t *testing.T) {
	a, repo, source := fixture(t)
	creationHookFiles(t, source.Path, []string{"never"}, map[string]string{
		"never.lua": `return function() require("wtf.fs").write(".wtf/local/ran", "unexpected") end`,
	})
	outside := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(outside, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source.Path, ".wtf/schema.json")); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(t.Context(), "../transfer-failed", "@")
	if ErrorCode(err) != "invalid_setup_path" || child.ID == "" {
		t.Fatalf("transfer failure: %+v %v", child, err)
	}
	state, err := repo.Load()
	if err != nil || state.Workspaces[child.ID].Status != "hook_failed" {
		t.Fatalf("failed workspace lost: %+v %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, ".jj")); err != nil {
		t.Fatal("failed workspace disappeared")
	}
	if _, err := os.Stat(filepath.Join(child.Path, ".wtf/local/ran")); !os.IsNotExist(err) {
		t.Fatal("creation hook ran after transfer failure")
	}
}
