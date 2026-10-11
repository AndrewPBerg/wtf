package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func invoke(t *testing.T, a *core.App, args ...string) (Envelope, int, string) {
	t.Helper()
	var out, logs bytes.Buffer
	app := core.New(a.Cwd, a.Home, &logs)
	app.Reviews = testReviewClient{}
	s := New(app, &out, &logs)
	code := s.Execute(context.Background(), args)
	var result Envelope
	dec := json.NewDecoder(&out)
	if e := dec.Decode(&result); e != nil {
		t.Fatalf("invalid JSON for %v: %v output=%q logs=%q", args, e, out.String(), logs.String())
	}
	var extra any
	if e := dec.Decode(&extra); e == nil {
		t.Fatalf("multiple stdout results for %v", args)
	}
	if result.Version != 1 {
		t.Fatalf("missing version %+v", result)
	}
	return result, code, logs.String()
}

type testReviewClient struct{}

func (testReviewClient) Resolve(_ context.Context, _ string, uri string) core.ReviewResolution {
	return core.ReviewResolution{Provider: "github", URL: uri, Code: "review_not_found"}
}

func (testReviewClient) Diagnose(context.Context, string) core.ReviewResolution {
	return core.ReviewResolution{Status: "ready", Workspace: "cascadinglabs", WorkspaceID: "test-workspace", Server: "linear"}
}
func TestEveryCommandAndAliasHasJSONHelpAndArgumentErrors(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	root := New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root
	root.InitDefaultHelpCmd()
	var walk func(*cobra.Command, []string)
	walk = func(c *cobra.Command, path []string) {
		if c.Short == "" {
			t.Fatal("missing help", path)
		}
		result, code, _ := invoke(t, a, append(append([]string{}, path...), "--json", "--help")...)
		if code != 0 || !result.OK || result.Data == nil {
			t.Fatalf("help %v: %+v %d", path, result, code)
		}
		for _, alias := range c.Aliases {
			if len(path) == 0 {
				continue
			}
			aliased := append([]string{}, path...)
			aliased[len(aliased)-1] = alias
			aliasResult, code, _ := invoke(t, a, append(aliased, "--json", "--help")...)
			if code != 0 || aliasResult.Command != result.Command {
				t.Fatalf("alias %s differs %+v", alias, aliasResult)
			}
		}
		result, code, _ = invoke(t, a, append(append([]string{}, path...), "--json", "--not-a-real-flag")...)
		if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("arguments %v: %+v %d", path, result, code)
		}
		for _, child := range c.Commands() {
			walk(child, append(append([]string{}, path...), child.Name()))
		}
	}
	walk(root, nil)
}
func TestJSONSuccessFailuresAndHookOutputIsolation(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	cfg := filepath.Join(dir, "jj.toml")
	if e := os.WriteFile(cfg, []byte("[user]\nname='CLI Test'\nemail='cli@example.test'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("JJ_CONFIG", cfg)
	cmd := exec.Command("jj", "git", "init", root)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("jj init %v %s", e, b)
	}
	if e := os.MkdirAll(filepath.Join(root, ".wtf"), 0700); e != nil {
		t.Fatal(e)
	}
	files := map[string]string{".gitignore": ".wtf/workspace.json\n", ".wtf/config.toml": "version=1\nlua='hooks.lua'\n[tasks.test]\ncommand=['sh','-c','printf task-output']\n", ".wtf/hooks.lua": `return {hooks={prepare=function(ctx) print("hook-output");ctx:stage("prepare",{"sh","-c","printf child-output"}) end, fail=function(ctx) ctx:stage("failure",{"sh","-c","exit 7"}) end}}`}
	for p, v := range files {
		if e := os.WriteFile(filepath.Join(root, p), []byte(v), 0600); e != nil {
			t.Fatal(e)
		}
	}
	a := core.New(root, filepath.Join(dir, "home"), nil)
	initializationID := ""
	success := [][]string{{"NeW", "."}, {"new", "."}, {"list"}, {"ls"}, {"list", "--here"}, {"list", "--global"}, {"ls", "default"}, {"doctor"}, {"hook"}, {"hook", "list"}, {"hook", "run", "prepare"}, {"run", "test"}, {"ports"}, {"ports", "list"}, {"ports", "claim", "web", "--port", "3000"}, {"ports", "release", "web"}, {"version"}, {"new", "../agent-a", "--from", "@"}, {"remove", "agent-a"}}
	for _, args := range success {
		result, code, logs := invoke(t, a, append(args, "--json")...)
		if code != 0 || !result.OK {
			t.Fatalf("success %v: %+v logs=%s", args, result, logs)
		}
		if len(args) == 2 && args[1] == "." {
			encoded, err := json.Marshal(result.Data)
			if err != nil {
				t.Fatal(err)
			}
			var initialized core.Initialization
			if err := json.Unmarshal(encoded, &initialized); err != nil {
				t.Fatal(err)
			}
			if initializationID == "" {
				if initialized.Code != "initialized" || initialized.ID == "" {
					t.Fatalf("missing first initialization result: %+v", initialized)
				}
				initializationID = initialized.ID
			} else if initialized.Code != "already_initialized" || initialized.ID != initializationID {
				t.Fatalf("repeated initialization changed identity or code: %+v", initialized)
			}
		}
		if len(args) >= 3 && args[0] == "hook" && args[2] == "prepare" {
			if !strings.Contains(logs, "hook-output") || !strings.Contains(logs, "child-output") {
				t.Fatalf("missing logs %q", logs)
			}
		}
	}
	failures := [][]string{{"new"}, {"hook", "run", "missing"}, {"hook", "run", "fail"}, {"run", "missing"}, {"ports", "claim", "bad", "--port", "70000"}, {"remove", "default"}, {"ls", "missing"}, {"doctor"}}
	for _, command := range []string{"list", "ls"} {
		for _, all := range []bool{false, true} {
			args := []string{command, "--json"}
			if all {
				args = append(args, "--all")
			}
			result, code, logs := invoke(t, a, args...)
			if code != 0 {
				t.Fatalf("list failed: %s", logs)
			}
			encoded, err := json.Marshal(result.Data)
			if err != nil {
				t.Fatal(err)
			}
			var rows []core.WorkspaceView
			if err := json.Unmarshal(encoded, &rows); err != nil {
				t.Fatal(err)
			}
			foundRemoved := false
			for _, row := range rows {
				if row.Name == "agent-a" && row.Status == "removed" {
					foundRemoved = true
				}
			}
			if foundRemoved != all {
				t.Fatalf("%s --all=%v: removed workspace visibility=%v", command, all, foundRemoved)
			}
		}
	}
	for _, args := range failures {
		result, code, _ := invoke(t, a, append(args, "--json")...)
		if code == 0 || result.OK || result.Error == nil || result.Error.Code == "" {
			t.Fatalf("failure %v: %+v", args, result)
		}
	}
}

func TestBareCommandShowsHelpWithoutRegistration(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	result, code, _ := invoke(t, a, "--json")
	if code != 0 || !result.OK || result.Command != "wtf" {
		t.Fatalf("bare command failed: %+v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["commands"] == nil || data["usage"] != "wtf" {
		t.Fatalf("bare command did not return help: %+v", result.Data)
	}
	encoded, err := json.Marshal(data["commands"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"name":"init"`) || strings.Contains(string(encoded), "Deprecated:") {
		t.Fatalf("removed init is still advertised: %s", encoded)
	}
	if entries, err := os.ReadDir(a.Home); err != nil || len(entries) != 0 {
		t.Fatalf("bare command registered state: %+v %v", entries, err)
	}
}

func TestInitIsNotACommand(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	root := New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root
	for _, command := range root.Commands() {
		if commandMatches(command, "init") {
			t.Fatal("init still exists in the command tree")
		}
	}
	for _, args := range [][]string{{"init", "--json"}, {"InIt", "-J"}} {
		result, code, _ := invoke(t, a, args...)
		if code == 0 || result.OK || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("removed command accepted: %v %+v", args, result)
		}
	}
}

func TestListHasOnlyLsAlias(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	result, code, _ := invoke(t, a, "LS", "--json", "--help")
	if code != 0 || result.Command != "wtf list" {
		t.Fatalf("ls no longer resolves list: %+v", result)
	}
	data := result.Data.(map[string]any)
	aliases, err := json.Marshal(data["aliases"])
	if err != nil || string(aliases) != `["ls"]` {
		t.Fatalf("unexpected list aliases: %s %v", aliases, err)
	}
	for _, alias := range []string{"switch", "sw", "w", "sww", "SWITCH", "Sw", "W"} {
		result, code, _ := invoke(t, a, alias, "--json")
		if code == 0 || result.OK || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("removed alias %s accepted: %+v", alias, result)
		}
	}
}
