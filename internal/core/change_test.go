package core

import (
	"os"
	"reflect"
	"testing"
)

func TestChangeWorkspacePreservesRegistryAndSkipsInitialization(t *testing.T) {
	a, r, anchor := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "../ChildCase", "@")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim(t.Context(), child.Workspace, "port", "web", "3000"); err != nil {
		t.Fatal(err)
	}
	config(t, anchor.Path, `return {overrides={init=function(ctx) ctx:block("Must not initialize") end}}`)
	for _, test := range []struct {
		name, message, id string
	}{
		{"ChildCase", "Picking this up Tomorrow 日本", child.ID},
		{child.ID, "", child.ID},
		{"", "Current workspace", anchor.ID},
	} {
		before, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		changed, err := a.ChangeWorkspace(t.Context(), test.name, test.message)
		if err != nil || changed.ID != test.id || changed.Message != test.message {
			t.Fatalf("change: %+v %v", changed, err)
		}
		after, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		expected := before.Workspaces[test.id]
		expected.Message = test.message
		before.Workspaces[test.id] = expected
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(changed, expected) {
			t.Fatal("change modified fields beyond the selected workspace message")
		}
	}
}

func TestChangeWorkspaceFailuresDoNotMutateRegistry(t *testing.T) {
	a, r, _ := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "../ChildCase", "@")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, message, code string
	}{
		{"childcase", "New", "workspace_missing"},
		{"missing", "New", "workspace_missing"},
		{"ChildCase", "bad\x00message", "invalid_message"},
	} {
		before, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.ChangeWorkspace(t.Context(), test.name, test.message)
		if ErrorCode(err) != test.code {
			t.Fatalf("%s: %v", test.name, err)
		}
		after, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatal("failed change mutated registry")
		}
	}
	if err := os.RemoveAll(child.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ChangeWorkspace(t.Context(), child.Name, "New"); ErrorCode(err) != "workspace_unavailable" {
		t.Fatalf("unavailable workspace: %v", err)
	}
	if err := r.Update(t.Context(), child.ID, func(w *Workspace) { w.Status = "removed" }); err != nil {
		t.Fatal(err)
	}
	before, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ChangeWorkspace(t.Context(), child.ID, "New"); ErrorCode(err) != "workspace_missing" {
		t.Fatalf("removed workspace: %v", err)
	}
	after, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("removed workspace mutated")
	}
}
