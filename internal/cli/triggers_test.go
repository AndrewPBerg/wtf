package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func triggerFixture(t *testing.T) *core.App {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	jjConfig := filepath.Join(base, "jj.toml")
	if err := os.WriteFile(jjConfig, []byte("[user]\nname='WTF Trigger Test'\nemail='triggers@example.test'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JJ_CONFIG", jjConfig)
	if output, err := exec.Command("jj", "git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("jj init: %v %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".wtf/workspace.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := core.New(root, filepath.Join(base, "home"), nil)
	if _, err := a.Setup(t.Context(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWorkspace(t.Context(), ".", ""); err != nil {
		t.Fatal(err)
	}
	return a
}

func installTriggerExamples(t *testing.T, a *core.App) {
	t.Helper()
	var config strings.Builder
	config.WriteString("version=1\n[tasks.hello]\ncommand=['sh','-c','true']\n")
	for _, scope := range []string{"new", "list", "remove", "invocation"} {
		fmt.Fprintf(&config, "\n[%s]\n", scope)
		for _, trigger := range []string{"before", "after", "failed", "finally"} {
			module := scope + "_" + trigger
			fmt.Fprintf(&config, "%s=['%s']\n", trigger, module)
			source := fmt.Sprintf(`local wtf=require("wtf")
return function(event)
  assert(event.surface == %q and event.trigger == %q)
  assert(event.command and event.request and event.ctx)
  if event.trigger == "failed" then assert(event.error and event.error.code and event.error.message) end
  if event.trigger == "after" then assert(event.error == nil and event.result ~= nil) end
  if event.surface == "remove" and event.trigger == "after" then
    assert(event.workspace.status == "removed")
    local ok, err = pcall(function() wtf.resources.list() end)
    assert(not ok and err.code == "workspace_unavailable")
  end
  wtf.notify("hello world " .. event.surface .. "." .. event.trigger)
end`, scope, trigger)
			if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", module+".lua"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte(config.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

func helloTriggerLines(logs string) []string {
	var result []string
	for _, line := range strings.Split(logs, "\n") {
		if _, suffix, ok := strings.Cut(line, "hello world "); ok {
			result = append(result, suffix)
		}
	}
	return result
}

// Every one of the 4 surfaces x 4 triggers is exercised through the real CLI.
func TestHelloWorldTriggerMatrix(t *testing.T) {
	for _, surface := range []string{"new", "list", "remove", "invocation"} {
		for _, failure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%v", surface, failure), func(t *testing.T) {
				a := triggerFixture(t)
				if surface == "remove" && !failure {
					if _, err := a.NewWorkspace(t.Context(), "../victim", "@"); err != nil {
						t.Fatal(err)
					}
				}
				installTriggerExamples(t, a)
				var args []string
				switch surface {
				case "new":
					args = []string{"NeW", "../created", "-F", "@", "--", "KeepCase"}
					if failure {
						args = []string{"new", "../failed", "-f", "missing-revision"}
					}
				case "list":
					args = []string{"LS", "--pr=false", "--checks=false"}
					if failure {
						args = append(args, "missing-workspace")
					}
				case "remove":
					args = []string{"RM", "victim"}
					if failure {
						args = []string{"remove", "default"}
					}
				case "invocation":
					args = []string{"run", "hello"}
					if failure {
						args = []string{"run", "missing-task"}
					}
				}
				// Put JSON before the separator so project argv stays untouched.
				args = append([]string{"-j"}, args...)
				result, code, logs := invoke(t, a, args...)
				if (code != 0) != failure || result.OK == failure {
					t.Fatalf("matrix operation: %+v code=%d logs=%s", result, code, logs)
				}
				var want []string
				want = append(want, "invocation.before")
				if surface != "invocation" {
					want = append(want, surface+".before")
				}
				phase := "after"
				if failure {
					phase = "failed"
				}
				if surface != "invocation" {
					want = append(want, surface+"."+phase)
				}
				want = append(want, "invocation."+phase)
				if surface != "invocation" {
					want = append(want, surface+".finally")
				}
				want = append(want, "invocation.finally")
				if got := helloTriggerLines(logs); !reflect.DeepEqual(got, want) {
					t.Fatalf("trigger order: got %v want %v logs=%q", got, want, logs)
				}
				if strings.Contains(logs, "\x1b") {
					t.Fatal("trigger logs styled in JSON mode")
				}
			})
		}
	}
}

func TestTriggerInspectionNeverRunsCallbacks(t *testing.T) {
	a := triggerFixture(t)
	installTriggerExamples(t, a)
	for _, args := range [][]string{{"--help", "-j"}, {"list", "--help", "-j"}, {"version", "-j"}, {"hook", "list", "-j"}, {"remove", "-j"}, {"setup", "teardown", "-j"}} {
		_, _, logs := invoke(t, a, args...)
		if len(helloTriggerLines(logs)) != 0 {
			t.Fatalf("inspection ran callbacks: %v %s", args, logs)
		}
	}
}

func TestTriggerBeforeBlocksMutationAndCleanupPreservesPrimaryError(t *testing.T) {
	a := triggerFixture(t)
	config := "version=1\n[new]\nbefore=['gate']\nfailed=['bad','hello_failed']\nfinally=['bad','hello_finally']\n"
	files := map[string]string{
		"gate.lua":          `return function(event) require("wtf.policy").block("primary guard failure") end`,
		"bad.lua":           `return function(event) require("wtf.policy").block("cleanup failure") end`,
		"hello_failed.lua":  `return function(event) assert(event.error.code=="hook_blocked"); require("wtf").notify("hello world failed") end`,
		"hello_finally.lua": `return function(event) assert(event.error.code=="hook_blocked"); require("wtf").notify("hello world finally") end`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	result, code, logs := invoke(t, a, "new", "../blocked", "-f", "@", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "hook_blocked" || !strings.Contains(result.Error.Message, "primary guard failure") || !strings.Contains(result.Error.Message, "cleanup failure") {
		t.Fatalf("cleanup replaced primary failure: %+v %s", result, logs)
	}
	if got := helloTriggerLines(logs); !reflect.DeepEqual(got, []string{"failed", "finally"}) {
		t.Fatalf("cleanup stopped early: %v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.Cwd), "blocked")); !os.IsNotExist(err) {
		t.Fatalf("before hook failed to block creation: %v", err)
	}
}

func TestTriggerFinallyRunsAfterCancellation(t *testing.T) {
	a := triggerFixture(t)
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte("version=1\n[invocation]\nfailed=['cleanup']\nfinally=['cleanup']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", "cleanup.lua"), []byte(`return function(event) require("wtf").notify("hello world "..event.trigger) end`), 0600); err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	a.Out = &logs
	ctx, cancel := context.WithCancel(context.Background())
	_, err := a.InvokeHooks(ctx, core.HookInvocation{Command: "run"}, func() (any, error) { cancel(); return nil, core.Fail("command_cancelled", "cancelled") })
	if core.ErrorCode(err) != "command_cancelled" || !reflect.DeepEqual(helloTriggerLines(logs.String()), []string{"failed", "finally"}) {
		t.Fatalf("cancelled cleanup: %v %q", err, logs.String())
	}
}

func TestTriggerAfterFailureRetainsCreatedWorkspace(t *testing.T) {
	a := triggerFixture(t)
	config := "version=1\n[new]\nafter=['block']\nfailed=['hello']\nfinally=['hello']\n"
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"block.lua": `return function(event) require("wtf.policy").block("post-creation failure") end`,
		"hello.lua": `return function(event) assert(event.result.id and event.workspace.id==event.result.id); require("wtf").notify("hello world "..event.trigger) end`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, code, logs := invoke(t, a, "new", "../retained", "-f", "@", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "hook_blocked" {
		t.Fatalf("after failure: %+v %s", result, logs)
	}
	if got := helloTriggerLines(logs); !reflect.DeepEqual(got, []string{"failed", "finally"}) {
		t.Fatalf("after failure skipped cleanup: %v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.Cwd), "retained")); err != nil {
		t.Fatalf("after failure removed workspace: %v", err)
	}
	r, err := core.Discover(t.Context(), a.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Select("retained")
	if err != nil || w.Status != "hook_failed" {
		t.Fatalf("retained state: %+v %v", w, err)
	}
}

func TestTriggerFinallyFailureDoesNotRedispatchFailed(t *testing.T) {
	a := triggerFixture(t)
	config := "version=1\n[tasks.hello]\ncommand=['sh','-c','true']\n[invocation]\nfailed=['unexpected']\nfinally=['block','hello']\n"
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"unexpected.lua": `return function(event) error("failed must not recurse after finally") end`,
		"block.lua":      `return function(event) require("wtf.policy").block("finally failure") end`,
		"hello.lua":      `return function(event) require("wtf").notify("hello world finally") end`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "hooks", name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, code, logs := invoke(t, a, "run", "hello", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "hook_blocked" || strings.Contains(result.Error.Message, "must not recurse") {
		t.Fatalf("finally recursion: %+v %s", result, logs)
	}
	if got := helloTriggerLines(logs); !reflect.DeepEqual(got, []string{"finally"}) {
		t.Fatalf("finally sequence stopped early: %v", got)
	}
}
