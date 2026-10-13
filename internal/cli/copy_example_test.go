package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyExampleSelectsCopiesLinksAndPreservesEdits(t *testing.T) {
	a := triggerFixture(t)
	root := a.Cwd
	script, err := os.ReadFile(filepath.Join("..", "..", "examples", "copy", "copy.lua"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, ".wtf/hooks/copy.lua"), script)
	write(filepath.Join(root, ".wtf/config.toml"), []byte("version=1\n[new]\nafter=['copy']\n"))
	result, code, logs := invoke(t, a, "new", "../missing-source", "-f", "@", "-j")
	if code != 0 || !result.OK || !strings.Contains(logs, "no local source directory") {
		t.Fatalf("missing source: %+v %s", result, logs)
	}
	local := ".wtf/local/copy-example"
	for _, name := range []string{".env.local", "copy me.md", "preferences.local"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "examples", "copy", "files", name))
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(root, local, name), data)
	}
	write(filepath.Join(root, local, "unselected.txt"), []byte("skip"))
	write(filepath.Join(root, local, "directory.local", "nested"), []byte("skip"))
	if err := os.Symlink(".env.local", filepath.Join(root, local, "source-link.local")); err != nil {
		t.Fatal(err)
	}
	result, code, logs = invoke(t, a, "new", "../copy-child", "-f", "@", "-j")
	if code != 0 || !result.OK || !strings.Contains(logs, "3 local files prepared") {
		t.Fatalf("copy creation: %+v %s", result, logs)
	}
	child := filepath.Join(filepath.Dir(root), "copy-child")
	for _, name := range []string{".env.local", "copy me.md"} {
		copied, err := os.ReadFile(filepath.Join(child, local, name))
		if err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile(filepath.Join(root, local, name))
		if err != nil || string(copied) != string(source) {
			t.Fatalf("copy mismatch %s", name)
		}
		info, err := os.Lstat(filepath.Join(child, local, name))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("copy is not regular %s", name)
		}
	}
	target, err := os.Readlink(filepath.Join(child, local, "preferences.local"))
	if err != nil || target != filepath.Join(root, local, "preferences.local") {
		t.Fatalf("link: %q %v", target, err)
	}
	entries, err := os.ReadDir(filepath.Join(child, local))
	if err != nil || len(entries) != 3 {
		t.Fatalf("selection: %v %v", entries, err)
	}
	write(filepath.Join(child, local, "copy me.md"), []byte("child edits"))
	result, code, logs = invoke(t, a, "new", "../copy-child", "-f", "@", "-j")
	if code != 0 || !result.OK || strings.Contains(logs, "local files prepared") {
		t.Fatalf("rerun: %+v error=%+v %s", result, result.Error, logs)
	}
	data, err := os.ReadFile(filepath.Join(child, local, "copy me.md"))
	if err != nil || string(data) != "child edits" {
		t.Fatal("child edits overwritten")
	}
	data, err = os.ReadFile(filepath.Join(root, local, "copy me.md"))
	if err != nil || string(data) == "child edits" {
		t.Fatal("copy mutated source")
	}
	a.Cwd = root
	result, code, logs = invoke(t, a, "hook", "run", "copy", "-j")
	if code != 0 || !result.OK || !strings.Contains(logs, "nothing to transfer") {
		t.Fatalf("main skip: %+v %s", result, logs)
	}
}
