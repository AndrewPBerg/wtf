package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestChangeMessageTargetingPersistenceAndCase(t *testing.T) {
	a := triggerFixture(t)
	child, err := a.NewWorkspace(t.Context(), "../ChildCase", "@")
	if err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(child.Path, "src")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		cwd  string
		args []string
		want string
	}{
		{a.Cwd, []string{"ChAnGe", "ChildCase", "--MeSsAgE", "Picking this up Tomorrow 日本"}, "Picking this up Tomorrow 日本"},
		{a.Cwd, []string{"CHANGE", "ChildCase", "-M", ""}, ""},
		{subdir, []string{"change", "-m", "Current from subdirectory"}, "Current from subdirectory"},
		{a.Cwd, []string{"change", child.ID, "-m", "By ID"}, "By ID"},
	} {
		app := core.New(test.cwd, a.Home, nil)
		result, code, logs := invoke(t, app, append(test.args, "-J")...)
		if code != 0 || result.Command != "wtf change" {
			t.Fatalf("%v: %+v %s", test.args, result, logs)
		}
		data, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		var w core.Workspace
		if err := json.Unmarshal(data, &w); err != nil || w.ID != child.ID || w.Message != test.want {
			t.Fatalf("unexpected change result: %s %v", data, err)
		}
		rows, err := core.New(a.Cwd, a.Home, nil).List(t.Context(), false, false)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			if row.ID == child.ID {
				found = row.Message == test.want
			}
		}
		if !found {
			t.Fatal("updated message did not persist for listing")
		}
	}
	for _, args := range [][]string{
		{"change", "childcase", "-m", "bad"},
		{"change", "missing", "-m", "bad"},
		{"change", "ChildCase", "extra", "-m", "bad"},
		{"change", "-m"},
		{"change", "-m", "bad\x00message"},
	} {
		result, code, _ := invoke(t, a, append([]string{"-j"}, args...)...)
		if code == 0 || result.Error == nil {
			t.Fatalf("accepted invalid change: %v %+v", args, result)
		}
	}
}

func TestChangeMessageDefaultsAndHumanOutput(t *testing.T) {
	a := triggerFixture(t)
	t.Setenv("NO_COLOR", "1")
	writeDefaults(t, filepath.Join(a.Home, "config.toml"), "[commands.change]\nmessage='Global'\n")
	writeDefaults(t, filepath.Join(a.Cwd, ".wtf", "config.toml"), "[commands.CHANGE]\nMESSAGE='Inherited Description'\n")
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"change"}, "Inherited Description"},
		{[]string{"change", "-m", "Explicit Description"}, "Explicit Description"},
		{[]string{"change", "-m", ""}, ""},
	} {
		var out, logs bytes.Buffer
		s := New(core.New(a.Cwd, a.Home, &logs), &out, &logs)
		code := s.Execute(t.Context(), test.args)
		verb := "Message updated for default"
		if test.want == "" {
			verb = "Message cleared for default"
		}
		if code != 0 || !strings.Contains(out.String(), verb) || strings.Contains(out.String(), "\x1b[") {
			t.Fatalf("human output: %d %q %s", code, out.String(), logs.String())
		}
		r, err := core.Discover(t.Context(), a.Cwd)
		if err != nil {
			t.Fatal(err)
		}
		w, err := r.Workspace(t.Context())
		if err != nil || w.Message != test.want {
			t.Fatalf("default precedence: %+v %v", w, err)
		}
	}
}

func TestChangeWithoutMessageShowsHelpAndSkipsHooks(t *testing.T) {
	a := triggerFixture(t)
	writeDefaults(t, filepath.Join(a.Cwd, ".wtf", "config.toml"), "[invocation]\nbefore=['block']\n")
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", "block.lua"), []byte(`return function(event) event.ctx:block("Mutation blocked") end`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"change"}, {"change", "default"}, {"change", "-m", "Help only", "--help"}} {
		result, code, logs := invoke(t, a, append(args, "-j")...)
		data, ok := result.Data.(map[string]any)
		if code != 0 || !ok || data["usage"] == nil || logs != "" {
			t.Fatalf("help: %+v %s", result, logs)
		}
	}
	result, code, _ := invoke(t, a, "change", "-m", "Blocked", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "hook_blocked" {
		t.Fatalf("mutation skipped invocation hooks: %+v", result)
	}
	r, err := core.Discover(t.Context(), a.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Workspace(t.Context())
	if err != nil || w.Message != "" {
		t.Fatalf("help or blocked mutation changed message: %+v %v", w, err)
	}
}
