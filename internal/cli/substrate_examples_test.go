package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelloWorldSubstrateExamplesRepeatAndPreserveEdits(t *testing.T) {
	a := triggerFixture(t)
	t.Setenv("WTF_HELLO_LABEL", "Test label with 'quotes' and $(literal text)")
	for _, name := range []string{"config.toml", "hello_world_new.lua", "hello_world_list.lua", "lib/hello_world.lua"} {
		source := filepath.Join("..", "..", "examples", "hello-world", name)
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(a.Cwd, ".wtf", "hooks", name)
		if name == "config.toml" {
			dest = filepath.Join(a.Cwd, ".wtf", name)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, child := range []string{"../hello-one", "../hello-two"} {
		result, code, logs := invoke(t, a, "new", child, "-f", "@", "-j")
		if code != 0 || !result.OK || !strings.Contains(logs, "hello world · new") {
			t.Fatalf("new example: %+v %s", result, logs)
		}
		dir := filepath.Join(a.Cwd, child, ".wtf", "local", "hello-world", "new")
		greeting, err := os.ReadFile(filepath.Join(dir, "greeting.txt"))
		if err != nil || !strings.Contains(string(greeting), "$(literal text)") {
			t.Fatalf("argv/env interpolation corrupted greeting: %q %v", greeting, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "latest")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "temporary-probe.txt")); !os.IsNotExist(err) {
			t.Fatal("temporary probe remains")
		}
	}
	dir := filepath.Join(a.Cwd, ".wtf", "local", "hello-world", "list")
	for i := 0; i < 3; i++ {
		result, code, logs := invoke(t, a, "LS", "--pr=false", "--checks=false", "-j")
		if code != 0 || !result.OK || !strings.Contains(logs, "hello world · list") {
			t.Fatalf("list example: %+v %s", result, logs)
		}
		if strings.Contains(logs, "\x1b") {
			t.Fatal("JSON hook logs are styled")
		}
		if i == 0 {
			if err := os.WriteFile(filepath.Join(dir, "kept-copy.txt"), []byte("my local edits"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		copy, err := os.ReadFile(filepath.Join(dir, "kept-copy.txt"))
		if err != nil || string(copy) != "my local edits" {
			t.Fatalf("repeat list replaced edits: %q %v", copy, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "temporary-probe.txt")); !os.IsNotExist(err) {
			t.Fatal("temporary probe remains")
		}
	}
	result, code, logs := invoke(t, a, "hook", "run", "hello_world_list", "-j")
	if code != 0 || !result.OK || !strings.Contains(logs, "hello world · list") {
		t.Fatalf("manual example: %+v %s", result, logs)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("repeat runs accumulated files: %v", entries)
	}
}
