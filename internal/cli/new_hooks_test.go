package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestDeclarativeCreationHookKeepsJSONClean(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	jjConfig := filepath.Join(base, "jj.toml")
	if err := os.WriteFile(jjConfig, []byte("[user]\nname='WTF Hook Test'\nemail='hooks@example.test'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JJ_CONFIG", jjConfig)
	if output, err := exec.Command("jj", "git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("jj init: %v %s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(root, ".wtf", "hooks"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		".gitignore":                 ".wtf/workspace.json\n",
		".wtf/config.toml":           "version=1\n[new]\nhooks=['hello_world']\n",
		".wtf/hooks/hello_world.lua": `local wtf = require("wtf"); return function(event) assert(event.args[1]=='ValueCase'); wtf.notify("hello world", "success") end`,
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := core.New(root, filepath.Join(base, "home"), nil)
	result, code, logs := invoke(t, a, "new", ".", "-j")
	if code != 0 || !result.OK || strings.Contains(logs, "hello world") {
		t.Fatalf("registration ran creation hook: %+v %s", result, logs)
	}
	result, code, logs = invoke(t, a, "NeW", "../hello-child", "-F", "@", "-J", "--", "ValueCase")
	if code != 0 || !result.OK || result.Command != "wtf new" || strings.Count(logs, "✓ hello world") != 1 || strings.Contains(logs, "\x1b") {
		t.Fatalf("creation hook output: %+v %s", result, logs)
	}
}
