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

func completionOutput(t *testing.T, app *core.App, words ...string) string {
	t.Helper()
	var out, logs bytes.Buffer
	session := New(app, &out, &logs)
	if code := session.Execute(context.Background(), append([]string{"__complete"}, words...)); code != 0 || logs.Len() != 0 {
		t.Fatalf("completion failed: %d %s %s", code, &out, &logs)
	}
	return out.String()
}

func TestCompletionScriptsAndJSONContracts(t *testing.T) {
	app := core.New(t.TempDir(), t.TempDir(), nil)
	if err := os.MkdirAll(filepath.Join(app.Cwd, ".wtf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app.Cwd, ".wtf/config.toml"), []byte("invalid ["), 0600); err != nil {
		t.Fatal(err)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		result, code, _ := invoke(t, app, "CoMpLeTiOn", strings.ToUpper(shell), "-J")
		if code != 0 || !result.OK || result.Command != "wtf completion" {
			t.Fatalf("%s: %+v", shell, result)
		}
		data, _ := json.Marshal(result.Data)
		var script completionScript
		if err := json.Unmarshal(data, &script); err != nil {
			t.Fatal(err)
		}
		if script.Shell != shell || script.Script != completionScripts[shell] {
			t.Fatalf("wrong script: %s", data)
		}
		var out, logs bytes.Buffer
		if code := New(app, &out, &logs).Execute(t.Context(), []string{"completion", shell}); code != 0 || out.String() != script.Script || logs.Len() != 0 {
			t.Fatalf("script contaminated: %d %s %s", code, &out, &logs)
		}
	}
	for _, args := range [][]string{{"completion", "-j"}, {"completion", "invalid", "-j"}, {"completion", "bash", "extra", "-j"}} {
		result, code, _ := invoke(t, app, args...)
		if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("invalid args accepted: %+v", result)
		}
	}
}

func TestCompletionSyntaxAndFutureCommands(t *testing.T) {
	app := core.New(t.TempDir(), t.TempDir(), nil)
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{[]string{"L"}, "ls\t"}, {[]string{"RM", "--J"}, "--json\t"},
		{[]string{"LiSt", "-P"}, "-p\t"}, {[]string{"HoOk", "R"}, "run\t"},
		{[]string{"completion", "F"}, "fish\t"}, {[]string{"LS", "--PR=F"}, "--PR=false\t"},
	} {
		got := completionOutput(t, app, tc.words...)
		if !strings.Contains(got, tc.want) || !strings.HasSuffix(got, ":4\n") {
			t.Fatalf("%v: %q", tc.words, got)
		}
	}
	var out, logs bytes.Buffer
	session := New(app, &out, &logs)
	group := &cobra.Command{Use: "future", Short: "Future extension", GroupID: "utilities"}
	leaf := &cobra.Command{Use: "inspect", Aliases: []string{"i"}, Short: "Inspect", Run: func(*cobra.Command, []string) { t.Fatal("executed future command") }}
	leaf.Flags().StringP("opaque", "o", "", "Opaque value")
	group.AddCommand(leaf)
	session.Root.AddCommand(group)
	if code := session.Execute(t.Context(), []string{"__complete", "FUTURE", "I"}); code != 0 || !strings.Contains(out.String(), "i\tInspect") {
		t.Fatalf("future alias missing: %s", &out)
	}
	out.Reset()
	if code := session.Execute(t.Context(), []string{"__complete", "FUTURE", "I", "--O"}); code != 0 || !strings.Contains(out.String(), "--opaque\t") {
		t.Fatalf("future flags missing: %s", &out)
	}
	if got := completionOutput(t, app, "run", "Test", "--", "--J"); got != ":0\n" {
		t.Fatalf("child syntax interpreted: %q", got)
	}
}

func TestCompletionLiveWorkspacesTasksAndReadOnlyScope(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	cmd := exec.Command("jj", "git", "init", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj init: %v %s", err, output)
	}
	app := core.New(root, filepath.Join(dir, "home"), nil)
	if _, err := app.NewWorkspace(t.Context(), ".", "@"); err != nil {
		t.Fatal(err)
	}
	repo, err := core.Discover(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(dir, "testing-msg")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := repo.Edit(t.Context(), func(state *core.State) error {
		state.Workspaces["test-id"] = core.Workspace{ID: "test-id", Name: "Testing-Msg", Path: workspacePath, Status: "active", Message: "this is me"}
		state.Workspaces["gone"] = core.Workspace{ID: "gone", Name: "Testing-Gone", Path: root, Status: "removed"}
		state.Workspaces["missing"] = core.Workspace{ID: "missing", Name: "Testing-Missing", Path: filepath.Join(dir, "absent"), Status: "active"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(repo.Store, "state.json")
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".wtf"), 0700); err != nil {
		t.Fatal(err)
	}
	config := "version=1\nlua='hooks.lua'\n[tasks.TestMixed]\ncommand=['echo','DoNotRun']\n"
	if err := os.WriteFile(filepath.Join(root, ".wtf/hooks.lua"), []byte("error('Completion executed project code')"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".wtf/config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	app.Cwd = sub
	for _, command := range []string{"LS", "RM", "change"} {
		got := completionOutput(t, app, command, "tmsg")
		if !strings.Contains(got, "Testing-Msg\tthis is me") || strings.Contains(got, "Gone") || strings.Contains(got, "Missing") {
			t.Fatalf("workspace candidates: %q", got)
		}
	}
	if got := completionOutput(t, app, "ls", "--here", ""); strings.Contains(got, "Testing-Msg") || !strings.Contains(got, "default\t") {
		t.Fatalf("current workspace scope: %q", got)
	}
	if got := completionOutput(t, app, "run", "tmix"); !strings.Contains(got, "TestMixed\techo DoNotRun") {
		t.Fatalf("task candidate: %q", got)
	}
	if got := completionOutput(t, app, "run", "TestMixed", ""); got != ":0\n" {
		t.Fatalf("extra task args: %q", got)
	}
	app.Cwd = dir
	if got := completionOutput(t, app, "ls", "test"); !strings.Contains(got, "Testing-Msg\t") || strings.Contains(got, "Gone") || strings.Contains(got, "Missing") {
		t.Fatalf("parent scope: %q", got)
	}
	app.Cwd = t.TempDir()
	if got := completionOutput(t, app, "ls", "test"); got != ":4\n" {
		t.Fatalf("unrelated scope: %q", got)
	}
	if got := completionOutput(t, app, "ls", "--GLOBAL", "test"); !strings.Contains(got, "Testing-Msg\t") {
		t.Fatalf("global scope: %q", got)
	}
	after, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("completion changed registry", err)
	}
}

func TestBashCompletionScriptCyclesFuzzyCandidates(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wtf")
	// A deterministic shell protocol fixture verifies the generated script actually
	// keeps non-prefix fuzzy candidates and registers the f alias.
	fixture := `#!/bin/sh
if [ "$1" != __complete ]; then exit 1; fi
if [ "$2" = ls ] && [ "$3" = tmsg ]; then
    printf 'Testing-Msg\tmessage\n:4\n'
elif [ "$2" = LS ] && [ "$3" = --PR=f ]; then
    printf '%s\t\n:4\n' --PR=false
else
    exit 1
fi
`
	if err := os.WriteFile(binary, []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	script := completionScripts["bash"] + "\nCOMP_WORDS=(f ls tmsg)\nCOMP_CWORD=2\n_wtf_complete\nprintf '%s\\n' \"${COMPREPLY[@]}\"\nCOMP_WORDS=(f LS --PR = f)\nCOMP_CWORD=4\n_wtf_complete\nprintf '%s\\n' \"${COMPREPLY[@]}\"\ncomplete -p f\n"
	cmd := exec.Command("bash", "--noprofile", "--norc")
	cmd.Env = append(os.Environ(), "WTF_BINARY="+binary)
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Testing-Msg\nfalse\n") || !strings.Contains(string(output), "-F _wtf_complete f") {
		t.Fatalf("bash completion: %v %s", err, output)
	}
}
