package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSetupCreatesIgnoredValidScaffoldAndPreservesEdits(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	a := core.New(root, home, nil)
	result, code, logs := invoke(t, a, "SeTuP", "-J")
	if code != 0 || !result.OK || result.Command != "wtf setup" || logs != "" {
		t.Fatalf("setup: %+v code=%d logs=%q", result, code, logs)
	}
	var first core.SetupResult
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &first); err != nil {
		t.Fatal(err)
	}
	names := []string{".wtf/hooks/", ".wtf/tasks/", ".wtf/.gitignore", ".wtf/schema.json", ".wtf/config.toml"}
	if first.Path != filepath.Join(root, ".wtf") || !reflect.DeepEqual(first.Created, names) || len(first.Existing) != 0 {
		t.Fatalf("unexpected scaffold report: %+v", first)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".wtf", ".gitignore"))
	if err != nil || string(ignore) != "*\n" {
		t.Fatalf("ignore rules: %q %v", ignore, err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, ".wtf", "tasks")); err != nil || len(entries) != 0 {
		t.Fatalf("tasks directory must start empty: %v %v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, ".wtf", "hooks")); err != nil || len(entries) != 0 {
		t.Fatalf("hooks directory must start empty: %v %v", entries, err)
	}
	cfg, err := core.LoadConfig(root)
	if err != nil || cfg.Version != 1 || cfg.Lua != "" {
		t.Fatalf("invalid scaffold configuration: %+v %v", cfg, err)
	}
	wantSchema, err := ConfigurationSchema(New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join(root, ".wtf", "schema.json"))
	if err != nil || !bytes.Equal(schema, wantSchema) {
		t.Fatalf("generated schema differs from command registry: %v", err)
	}
	if b, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	ignored, err := exec.Command("git", append([]string{"-C", root, "check-ignore", "--"}, names...)...).Output()
	if err != nil || !reflect.DeepEqual(strings.Fields(string(ignored)), names) {
		t.Fatalf("all scaffold files must be ignored: %q %v", ignored, err)
	}
	// A custom hook must neither be overwritten nor executed by repeat setup.
	edits := map[string]string{
		".wtf/config.toml":    "version = 1\nlua = \"hooks/init.lua\"\n# user edit\n",
		".wtf/hooks/init.lua": "error(\"setup executed project code\")\n",
		".wtf/schema.json":    "{\"user\": true}\n",
		".wtf/.gitignore":     "*\n# user edit\n",
	}
	for name, body := range edits {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, code, _ = invoke(t, a, "setup", "--JsOn=tRuE")
	if code != 0 || !result.OK {
		t.Fatalf("repeat setup: %+v", result)
	}
	encoded, err = json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var repeated core.SetupResult
	if err := json.Unmarshal(encoded, &repeated); err != nil {
		t.Fatal(err)
	}
	if len(repeated.Created) != 0 || !reflect.DeepEqual(repeated.Existing, names) {
		t.Fatalf("repeat setup report: %+v", repeated)
	}
	for name, body := range edits {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(got) != body {
			t.Fatalf("setup changed %s: %q %v", name, got, err)
		}
	}
	if err := os.Remove(filepath.Join(root, ".wtf", "schema.json")); err != nil {
		t.Fatal(err)
	}
	result, code, _ = invoke(t, a, "setup", "-j")
	if code != 0 || !result.OK {
		t.Fatalf("fill missing schema: %+v", result)
	}
	schema, err = os.ReadFile(filepath.Join(root, ".wtf", "schema.json"))
	if err != nil || !bytes.Equal(schema, wantSchema) {
		t.Fatalf("missing schema was not regenerated: %v", err)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("setup wrote global workspace state: %v %v", entries, err)
	}
}

func TestSetupRejectsPathConflictsWithoutFollowingSymlinks(t *testing.T) {
	for _, name := range []string{".wtf", ".wtf/hooks", ".wtf/tasks", ".wtf/config.toml"} {
		for _, symlink := range []bool{false, true} {
			t.Run(name+"/symlink="+map[bool]string{false: "false", true: "true"}[symlink], func(t *testing.T) {
				root, target := t.TempDir(), t.TempDir()
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				var err error
				switch {
				case symlink:
					err = os.Symlink(target, path)
				case name == ".wtf/config.toml":
					err = os.Mkdir(path, 0755)
				default:
					err = os.WriteFile(path, []byte("preserve me"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				_, err = core.New(root, t.TempDir(), nil).Setup(context.Background(), []byte("{}\n"))
				if core.ErrorCode(err) != "invalid_setup_path" {
					t.Fatalf("accepted conflicting path: %v", err)
				}
				if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
					t.Fatalf("setup wrote through symlink: %v %v", entries, err)
				}
			})
		}
	}
}

func TestSetupHumanOutputAndArgumentFailures(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	var out, logs bytes.Buffer
	if code := New(a, &out, &logs).Execute(context.Background(), []string{"setup"}); code != 0 {
		t.Fatalf("human setup: %s %s", out.String(), logs.String())
	}
	if !strings.Contains(out.String(), "WTF configuration ready") || !strings.Contains(out.String(), "Created") || strings.Contains(out.String(), `"created"`) || strings.Contains(out.String(), "\x1b") || logs.Len() != 0 {
		t.Fatalf("unexpected human output: %q logs=%q", out.String(), logs.String())
	}
	for _, args := range [][]string{{"setup", "extra", "-j"}, {"SETUP", "--unknown", "-J"}} {
		result, code, _ := invoke(t, a, args...)
		if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("arguments %v: %+v", args, result)
		}
	}
}

func TestSetupTeardownNeverDeletesWithoutInteractiveConfirmation(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	if _, err := a.Setup(context.Background(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	// Broken configuration must not hide cleanup or execute project code.
	config := filepath.Join(a.Cwd, ".wtf", "config.toml")
	if err := os.WriteFile(config, []byte("version = [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, code, _ := invoke(t, a, "SeTuP", "TeArDoWn", "-J")
	if code == 0 || result.Command != "wtf setup teardown" || result.Error == nil || result.Error.Code != "confirmation_required" {
		t.Fatalf("JSON teardown: %+v", result)
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("JSON teardown removed config: %v", err)
	}
	var out, logs bytes.Buffer
	if code := New(a, &out, &logs).Execute(context.Background(), []string{"setup", "teardown"}); code == 0 || !strings.Contains(logs.String(), "interactive terminal") {
		t.Fatalf("redirected teardown: %s %s", out.String(), logs.String())
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("redirected teardown removed config: %v", err)
	}
	result, code, _ = invoke(t, a, "setup", "teardown", "extra", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
		t.Fatalf("teardown arguments: %+v", result)
	}
	missing := core.New(t.TempDir(), t.TempDir(), nil)
	result, code, _ = invoke(t, missing, "setup", "teardown", "--json")
	if code != 0 || !result.OK {
		t.Fatalf("missing configuration: %+v", result)
	}
	setup, _, err := New(a, &out, &logs).Root.Find([]string{"setup"})
	if err != nil || setup.GroupID != "utilities" {
		t.Fatalf("setup belongs in Utilities: %v %v", setup, err)
	}
}

func TestSetupTeardownPromptDefaultsToNoAndRequiresEnter(t *testing.T) {
	p := humanPresentation(&bytes.Buffer{})
	for _, tc := range []struct {
		name      string
		keys      []tea.KeyMsg
		confirmed bool
	}{
		{"default-no", []tea.KeyMsg{{Type: tea.KeyEnter}}, false},
		{"explicit-no", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("n")}, {Type: tea.KeyEnter}}, false},
		{"yes", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("Y")}, {Type: tea.KeyEnter}}, true},
		{"select-yes", []tea.KeyMsg{{Type: tea.KeyRight}, {Type: tea.KeyEnter}}, true},
		{"escape-after-yes", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("y")}, {Type: tea.KeyEsc}}, false},
		{"interrupt", []tea.KeyMsg{{Type: tea.KeyCtrlC}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := setupTeardownPrompt{path: "/project/.wtf", p: p}
			if !strings.Contains(m.View(), "[No]") || !strings.Contains(m.View(), "/project/.wtf") {
				t.Fatalf("missing default and target: %s", m.View())
			}
			for i, key := range tc.keys {
				model, cmd := m.Update(key)
				m = model.(setupTeardownPrompt)
				if i < len(tc.keys)-1 && (cmd != nil || m.confirmed) {
					t.Fatal("confirmation completed before Enter")
				}
				if i == len(tc.keys)-1 && cmd == nil {
					t.Fatal("final decision did not close prompt")
				}
			}
			if m.confirmed != tc.confirmed {
				t.Fatalf("confirmed=%v want %v", m.confirmed, tc.confirmed)
			}
		})
	}
}
