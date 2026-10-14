package core

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*App, Repo, Workspace) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(dir, "jj.toml")
	if e := os.WriteFile(cfg, []byte("[user]\nname = 'WTF Test'\nemail = 'wtf@example.test'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("JJ_CONFIG", cfg)
	cmd := exec.Command("jj", "git", "init", root)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("init: %v %s", e, out)
	}
	if e := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".wtf/workspace.json\n*.count\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "input.txt"), []byte("first"), 0600); e != nil {
		t.Fatal(e)
	}
	a := New(root, filepath.Join(dir, "home"), io.Discard)
	w, e := a.Init(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	r, e := Discover(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	return a, r, w
}
func config(t *testing.T, root, lua string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Join(root, ".wtf"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, ".wtf", "config.toml"), []byte("version = 1\nlua = 'hooks.lua'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, ".wtf", "hooks.lua"), []byte(lua), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestStagesReuseOnlyMatchingInputsAndReadiness(t *testing.T) {
	a, r, w := fixture(t)
	ctx := context.Background()
	cmd := []string{"sh", "-c", "printf x >> prepare.count"}
	opts := StageOptions{Inputs: []string{"input.txt"}, Reuse: true}
	first, e := a.Stage(ctx, r, w, "prepare", cmd, opts)
	if e != nil || first.Reused {
		t.Fatalf("first: %+v %v", first, e)
	}
	second, e := a.Stage(ctx, r, w, "prepare", cmd, opts)
	if e != nil || !second.Reused {
		t.Fatalf("second: %+v %v", second, e)
	}
	if e = os.WriteFile(filepath.Join(w.Path, "input.txt"), []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	third, e := a.Stage(ctx, r, w, "prepare", cmd, opts)
	if e != nil || third.Reused {
		t.Fatalf("third: %+v %v", third, e)
	}
	b, e := os.ReadFile(filepath.Join(w.Path, "prepare.count"))
	if e != nil || string(b) != "xx" {
		t.Fatalf("runs: %s %v", b, e)
	}
	opts.Check = []string{"sh", "-c", "test -f ready.count"}
	_, e = a.Stage(ctx, r, w, "service", []string{"sh", "-c", "printf x >> start.count; touch ready.count"}, opts)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(w.Path, "ready.count")); e != nil {
		t.Fatal(e)
	}
	st, e := a.Stage(ctx, r, w, "service", []string{"sh", "-c", "printf x >> start.count; touch ready.count"}, opts)
	if e != nil || st.Reused {
		t.Fatalf("lost readiness: %+v %v", st, e)
	}
}
func TestFailureStopsLaterStagesAndRetryRetainsSetup(t *testing.T) {
	a, r, w := fixture(t)
	config(t, w.Path, `return { hooks = { prepare = function(ctx)
 ctx:stage("prepare", {"sh", "-c", "printf x >> prepare.count"}, {inputs={"input.txt"}, reuse=true})
 ctx:stage("test", {"sh", "-c", "test -f fixed.count"})
 ctx:stage("after", {"sh", "-c", "touch after.count"})
end } }`)
	if _, e := a.Hook(context.Background(), "prepare"); e == nil {
		t.Fatal("expected failure")
	}
	s, e := r.Load()
	if e != nil {
		t.Fatal(e)
	}
	if s.Workspaces[w.ID].Stages["prepare"].Status != "succeeded" || s.Workspaces[w.ID].Stages["test"].Status != "failed" {
		t.Fatalf("stages %+v", s.Workspaces[w.ID].Stages)
	}
	if _, ok := s.Workspaces[w.ID].Stages["after"]; ok {
		t.Fatal("later stage ran")
	}
	if e = os.WriteFile(filepath.Join(w.Path, "fixed.count"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	out, e := a.Hook(context.Background(), "prepare")
	if e != nil {
		t.Fatal(e)
	}
	if !out.Stages["prepare"].Reused || out.Status != "ready" {
		t.Fatalf("retry %+v", out)
	}
	b, e := os.ReadFile(filepath.Join(w.Path, "prepare.count"))
	if e != nil || string(b) != "x" {
		t.Fatalf("setup repeated %s %v", b, e)
	}
}
func TestCancellationPersistsFailureAndReleasesExecutionLock(t *testing.T) {
	a, r, w := fixture(t)
	config(t, w.Path, `return { hooks = { slow=function(ctx) ctx:stage("slow", {"sh", "-c", "sleep 30"}) end } }`)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, e := a.Hook(ctx, "slow"); e == nil {
		t.Fatal("expected cancellation")
	}
	s, e := r.Load()
	if e != nil {
		t.Fatal(e)
	}
	if s.Workspaces[w.ID].Stages["slow"].Status != "failed" {
		t.Fatalf("failure not persisted %+v", s)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	unlock, e := r.LockWorkspace(ctx2, w.ID)
	if e != nil {
		t.Fatal(e)
	}
	unlock()
}
func TestAtomicClaimsAcrossWorkspaces(t *testing.T) {
	_, r, w := fixture(t)
	id, e := ID()
	if e != nil {
		t.Fatal(e)
	}
	other := w
	other.ID = id
	other.Name = "other"
	if e = r.Edit(context.Background(), func(s *State) error { s.Workspaces[id] = other; return nil }); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, owner := range []Workspace{w, other} {
		wg.Add(1)
		go func(owner Workspace) {
			defer wg.Done()
			_, e := r.Claim(context.Background(), owner, "port", "web", "127.0.0.1:3000")
			results <- e
		}(owner)
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		switch {
		case e == nil:
			ok++
		case ErrorCode(e) == "claim_conflict":
			conflict++
		default:
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", ok, conflict)
	}
}
func TestImplicitOverrideDelegatesCreationAndTeardown(t *testing.T) {
	a, r, w := fixture(t)
	config(t, w.Path, `return { overrides = {
 new=function(ctx, req, default) local w=default(req); ctx:stage("prepare", {"sh","-c","touch prepared.count"}); ctx:claim("fixture","service", w.id); return w end,
 remove=function(ctx, req, default) ctx:stage("teardown", {"sh","-c","touch stopped.count"});ctx:release("fixture","service");return default(req) end
} }`)
	child, e := a.NewWorkspace(context.Background(), "../agent-a", "@")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(child.Path, "prepared.count")); e != nil {
		t.Fatal(e)
	}
	claims, e := r.Claims(child.Workspace)
	if e != nil || len(claims) != 1 {
		t.Fatalf("claims %+v %v", claims, e)
	}
	removed, e := a.Remove(context.Background(), child.Name)
	if e != nil {
		t.Fatal(e)
	}
	if removed.Status != "removed" {
		t.Fatalf("removed %+v", removed)
	}
	if _, e = os.Stat(child.Path); !os.IsNotExist(e) {
		t.Fatalf("directory remained: %v", e)
	}
	claims, e = r.Claims(child.Workspace)
	if e != nil || len(claims) != 0 {
		t.Fatalf("claims remain %+v %v", claims, e)
	}
}
func TestRemovalRefusesWrongIdentityAndChangedSource(t *testing.T) {
	a, _, _ := fixture(t)
	child, e := a.NewWorkspace(context.Background(), "../protected", "@")
	if e != nil {
		t.Fatal(e)
	}
	marker := workspaceMarkerPath(child.Path)
	original, e := os.ReadFile(marker)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(marker, []byte(`{"id":"someone-else"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Remove(context.Background(), child.Name); ErrorCode(e) != "identity_mismatch" {
		t.Fatalf("wrong marker %v", e)
	}
	if e = os.WriteFile(marker, original, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("important edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Remove(context.Background(), child.Name); ErrorCode(e) != "workspace_has_changes" {
		t.Fatalf("changed source %v", e)
	}
	if _, e = os.Stat(child.Path); e != nil {
		t.Fatal("workspace was lost", e)
	}
}
func TestInspectionDoesNotMutateAndValidationDoesNotExecute(t *testing.T) {
	a, r, w := fixture(t)
	config(t, w.Path, `error("must not execute during validate")`)
	before, e := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Validate(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = a.List(context.Background(), false, false); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if e != nil || string(before) != string(after) {
		t.Fatal("inspection mutated registry")
	}
}
func TestInputTraversalAndInvalidReuseRejected(t *testing.T) {
	a, r, w := fixture(t)
	if _, e := a.Stage(context.Background(), r, w, "bad", []string{"true"}, StageOptions{Reuse: true}); ErrorCode(e) != "invalid_reuse" {
		t.Fatalf("reuse %v", e)
	}
	if _, e := a.Stage(context.Background(), r, w, "bad", []string{"true"}, StageOptions{Inputs: []string{"../escape"}}); ErrorCode(e) != "invalid_path" {
		t.Fatalf("path %v", e)
	}
	if e := CheckName("../escape"); e == nil {
		t.Fatal("invalid name accepted")
	}
	if _, e := LoadConfig(w.Path); e != nil {
		t.Fatal(e)
	}
}
func TestHookCannotCallDefaultTwice(t *testing.T) {
	a, _, w := fixture(t)
	config(t, w.Path, `return {overrides={new=function(ctx,req,default) default(req);default(req) end}}`)
	child, e := a.NewWorkspace(context.Background(), "../once", "@")
	if ErrorCode(e) != "default_already_called" {
		t.Fatalf("expected guard %v", e)
	}
	if child.ID == "" {
		t.Fatal("lost created identity")
	}
	if !strings.HasSuffix(child.Path, "/once") {
		t.Fatal(child.Path)
	}
}

func TestLuaLoopRespondsToCancellation(t *testing.T) {
	a, _, w := fixture(t)
	config(t, w.Path, `return {hooks={loop=function(ctx) while true do end end}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := a.Hook(ctx, "loop"); ErrorCode(err) != "hook_cancelled" {
		t.Fatalf("expected bounded Lua cancellation, got %v", err)
	}
}
