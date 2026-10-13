package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceMarkerNeverBecomesSourceWithoutIgnoreRule(t *testing.T) {
	a, _, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(a.Cwd, ".gitignore"), []byte("*.count\n"), 0600); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(t.Context(), "../fresh", "@")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(workspaceMarkerPath(child.Path)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(child.Path, ".wtf", "workspace.json")); !os.IsNotExist(err) {
		t.Fatalf("source-tree marker created: %v", err)
	}
	if _, err = output(t.Context(), child.Path, "status"); err != nil {
		t.Fatal(err)
	}
	diff, err := output(t.Context(), child.Path, "diff", "--ignore-working-copy", "--summary")
	if err != nil || diff != "" {
		t.Fatalf("fresh workspace has source changes: %q %v", diff, err)
	}
	candidate := removalCandidateByID(t, a, child.ID)
	if len(candidate.Changes) != 0 || candidate.ObservedEdits || !candidate.CanRemove(false) {
		t.Fatalf("fresh workspace requires force: %+v", candidate)
	}
	if _, err = a.RemoveWithOptions(t.Context(), child.ID, candidate.RemovalOptions(false)); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMarkerLegacyIdentityRemainsSupported(t *testing.T) {
	a, r, _ := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "../legacy", "@")
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(child.Path, ".wtf", "workspace.json")
	if err = os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(workspaceMarkerPath(child.Path))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(workspaceMarkerPath(child.Path)); err != nil {
		t.Fatal(err)
	}
	if err = verifyIdentity(t.Context(), r, child.Workspace); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(workspaceMarkerPath(child.Path), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = verifyIdentity(t.Context(), r, child.Workspace); ErrorCode(err) != "identity_mismatch" {
		t.Fatalf("invalid canonical marker fell back to legacy: %v", err)
	}
}
