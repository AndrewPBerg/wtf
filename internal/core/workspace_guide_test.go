package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestWorkspaceGuideExamples(t *testing.T) {
	blocks := regexp.MustCompile("(?ms)^```(lua|toml|json)\\n(.*?)^```$").FindAllSubmatch(workspaceGuide, -1)
	if len(blocks) == 0 {
		t.Fatal("guide has no code examples")
	}
	L := newLua(t.Context())
	defer L.Close()
	var prepare, allocate, command string
	for _, block := range blocks {
		language, source := string(block[1]), string(block[2])
		switch language {
		case "lua":
			if _, err := L.LoadString(source); err != nil {
				t.Fatalf("invalid Lua example: %v\n%s", err, source)
			}
			if strings.Contains(source, "-- .wtf/hooks/prepare.lua") {
				prepare = source
			}
			if strings.Contains(source, "local function choose_web_port()") {
				allocate = source
			}
			if strings.Contains(source, "local result = exec.run(") {
				command = "return function(event)\n" + source + "\nend"
			}
		case "toml":
			var cfg Config
			if err := toml.Unmarshal([]byte(source), &cfg); err != nil {
				t.Fatalf("invalid TOML example: %v", err)
			}
		case "json":
			if !json.Valid([]byte(source)) {
				t.Fatal("invalid JSON example")
			}
		}
	}
	if prepare == "" || allocate == "" || command == "" {
		t.Fatal("missing runnable preparation, port, or command recipe")
	}
	a, r, w := fixture(t)
	if _, err := r.Claim(t.Context(), w, "port", "web", "127.0.0.1:39000"); err != nil {
		t.Fatal(err)
	}
	packageFiles(t, w.Path, map[string]string{
		"init.lua": `return require("wtf").setup {hooks = {
prepare = require("prepare"), allocate = require("allocate"), command = require("command")}}`,
		"prepare.lua":  prepare,
		"allocate.lua": allocate,
		"command.lua":  command,
	})
	for range 2 {
		for _, name := range []string{"prepare", "allocate", "command"} {
			if _, err := a.Hook(t.Context(), name); err != nil {
				t.Fatalf("guide recipe %s: %v", name, err)
			}
		}
	}
	for name, expected := range map[string]string{
		".wtf/local/prepared.txt": "ready\n",
		".wtf/web.env":            "PORT=39000\n",
	} {
		contents, err := os.ReadFile(filepath.Join(w.Path, name))
		if err != nil || string(contents) != expected {
			t.Fatalf("recipe artifact %s: %q %v", name, contents, err)
		}
	}
}

func TestWorkspaceGuideTransferWithoutRevisionTracking(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	if _, err := New(source, t.TempDir(), nil).Setup(t.Context(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	custom := "# Untracked project guide\n"
	if err := os.WriteFile(filepath.Join(source, ".wtf/AGENTS.md"), []byte(custom), 0600); err != nil {
		t.Fatal(err)
	}
	if err := inheritWorkspaceConfig(t.Context(), source, destination); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, ".wtf/AGENTS.md"))
	if err != nil || string(contents) != custom {
		t.Fatalf("untracked guide was not transferred: %q %v", contents, err)
	}
	if err := os.WriteFile(filepath.Join(destination, ".wtf/AGENTS.md"), []byte("# Child edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(filepath.Join(source, ".wtf/AGENTS.md"))
	if err != nil || string(contents) != custom {
		t.Fatal("guide was linked instead of independently copied")
	}
	if err := os.Remove(filepath.Join(source, ".wtf/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "guide.md")
	if err := os.WriteFile(outside, []byte("# External guide\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, ".wtf/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := inheritWorkspaceConfig(t.Context(), source, destination); ErrorCode(err) != "invalid_setup_path" {
		t.Fatalf("followed symlinked source guide: %v", err)
	}
	contents, err = os.ReadFile(filepath.Join(destination, ".wtf/AGENTS.md"))
	if err != nil || string(contents) != "# Child edit\n" {
		t.Fatal("unsafe transfer changed destination guide")
	}
}
