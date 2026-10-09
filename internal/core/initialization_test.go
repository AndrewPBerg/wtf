package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndNewSharePreparationWithIndependentOwnershipAndReuse(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `return { hooks = { init = function(ctx, request)
 ctx:stage("prepare", {"sh", "-c", "printf x >> prepare.count"}, {inputs={"input.txt"}, reuse=true})
 ctx:stage("arguments", {"sh", "-c", "printf '%s' \"$1\" > args.count", "sh", request.args[1] or ""})
end } }`)
	ctx := context.Background()
	first, err := a.Initialize(ctx, "ParentValue")
	if err != nil || first.Code != "already_initialized" || first.ID != anchor.ID || first.Owned || first.Stages["prepare"].Status != "succeeded" {
		t.Fatalf("initialize existing: %+v %v", first, err)
	}
	child, err := a.NewWorkspace(ctx, "feature", "@", "ChildValue")
	if err != nil || !child.Owned || child.ID == anchor.ID || child.Stages["prepare"].Status != "succeeded" || child.Stages["prepare"].Reused {
		t.Fatalf("new did not initialize an independent owned workspace: %+v %v", child, err)
	}
	for _, check := range []struct{ path, value string }{{anchor.Path, "ParentValue"}, {child.Path, "ChildValue"}} {
		count, err := os.ReadFile(filepath.Join(check.path, "prepare.count"))
		if err != nil || string(count) != "x" {
			t.Fatalf("setup did not run exactly once at %s: %q %v", check.path, count, err)
		}
		args, err := os.ReadFile(filepath.Join(check.path, "args.count"))
		if err != nil || string(args) != check.value {
			t.Fatalf("initialization arguments or context incorrect: %q %v", args, err)
		}
	}
	childApp := New(child.Path, a.Home, a.Out)
	repeated, err := childApp.Initialize(ctx, "ChildValue")
	if err != nil || repeated.ID != child.ID || !repeated.Owned || !repeated.Stages["prepare"].Reused {
		t.Fatalf("reinitialization lost ownership or setup reuse: %+v %v", repeated, err)
	}
	count, err := os.ReadFile(filepath.Join(child.Path, "prepare.count"))
	if err != nil || string(count) != "x" {
		t.Fatalf("reinitialization reran matching setup: %q %v", count, err)
	}
	state, err := r.Load()
	if err != nil || len(state.Workspaces) != 2 {
		t.Fatalf("initialization duplicated registrations: %+v %v", state, err)
	}
}

func TestFailedNewInitializationCanResumeThroughInit(t *testing.T) {
	a, _, anchor := fixture(t)
	config(t, anchor.Path, `return { hooks = { init = function(ctx)
 ctx:stage("prepare", {"sh", "-c", "printf x >> prepare.count"}, {inputs={"input.txt"}, reuse=true})
 ctx:stage("verify", {"sh", "-c", "test -f fixed.count"})
 ctx:stage("after", {"sh", "-c", "touch after.count"})
end } }`)
	ctx := context.Background()
	child, err := a.NewWorkspace(ctx, "retry", "@")
	if err == nil || child.ID == "" || !child.Owned || child.Stages["prepare"].Status != "succeeded" || child.Stages["verify"].Status != "failed" {
		t.Fatalf("new did not retain failed initialization: %+v %v", child, err)
	}
	if _, err := os.Stat(filepath.Join(child.Path, "after.count")); !os.IsNotExist(err) {
		t.Fatal("initialization continued after failure")
	}
	if err := os.WriteFile(filepath.Join(child.Path, "fixed.count"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	childApp := New(child.Path, a.Home, a.Out)
	retried, err := childApp.Initialize(ctx)
	if err != nil || retried.ID != child.ID || !retried.Owned || retried.Status != "ready" || !retried.Stages["prepare"].Reused || retried.Stages["after"].Status != "succeeded" {
		t.Fatalf("init did not resume new's failed setup: %+v %v", retried, err)
	}
}
