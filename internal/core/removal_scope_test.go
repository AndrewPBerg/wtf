package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParentRemovalScopesResolvesAndRunsRepositoryHooks(t *testing.T) {
	a, repo, _ := fixture(t)
	parent := filepath.Dir(repo.Root)
	second := filepath.Join(parent, "second")
	if _, err := output(t.Context(), parent, "git", "init", second); err != nil {
		t.Fatal(err)
	}
	b := *a
	b.Cwd = second
	if _, err := b.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	firstChild, err := a.NewWorkspace(t.Context(), "../first-child", "@")
	if err != nil {
		t.Fatal(err)
	}
	secondChild, err := b.NewWorkspace(t.Context(), "../second-child", "@")
	if err != nil {
		t.Fatal(err)
	}
	// A repository outside the parent must never enter removal's scope.
	outside := b
	outside.Cwd = filepath.Join(t.TempDir(), "outside")
	if _, err := output(t.Context(), parent, "git", "init", outside.Cwd); err != nil {
		t.Fatal(err)
	}
	if _, err := outside.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	parentPhaseFiles(t, repo.Root, "version=1\n[remove]\nbefore=['record']\n", map[string]string{
		"record.lua": `return function(event) require("wtf.fs").write("remove.count", "called", {root="project"}) end`,
	})
	scoped := *a
	scoped.Cwd = parent
	rows, err := scoped.RemovalCandidates(t.Context())
	if err != nil || len(rows) != 4 {
		t.Fatalf("parent preview: %+v %v", rows, err)
	}
	for _, row := range rows {
		if !row.Owned && row.CanRemove(true) {
			t.Fatalf("anchor became removable: %+v", row)
		}
	}
	if _, err := scoped.Remove(t.Context(), "default"); ErrorCode(err) != "ambiguous_workspace" {
		t.Fatalf("ambiguous name: %v", err)
	}
	// An explicit unique name dispatches to the correct repository lifecycle.
	if _, err := scoped.Remove(t.Context(), firstChild.Name); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(repo.Root, "remove.count")); err != nil || string(data) != "called" {
		t.Fatalf("repository removal hook: %q %v", data, err)
	}
	if _, err := os.Stat(firstChild.Path); !os.IsNotExist(err) {
		t.Fatalf("first child remains: %v", err)
	}
	if _, err := os.Stat(secondChild.Path); err != nil {
		t.Fatalf("wrong repository changed: %v", err)
	}
	// Parent picker proofs route by ID and retain the original safety checks.
	inspector, err := scoped.RemovalInspector(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID != secondChild.ID {
			continue
		}
		candidate, err := inspector.Inspect(t.Context(), row.WorkspaceView)
		if err != nil || !candidate.CanRemove(false) {
			t.Fatalf("second repository inspection: %+v %v", candidate, err)
		}
		if err := os.WriteFile(filepath.Join(secondChild.Path, "uncommitted.txt"), []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := scoped.RemoveWithOptions(t.Context(), row.ID, candidate.RemovalOptions(false)); ErrorCode(err) != "workspace_has_changes" {
			t.Fatalf("late edit was not protected: %v", err)
		}
	}
}
