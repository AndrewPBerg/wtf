package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWorkspaceInheritsNativeGitColocation(t *testing.T) {
	a, _, anchor := fixture(t)
	// Explicitly test JJ's native default without depending on user configuration.
	cfg := os.Getenv("JJ_CONFIG")
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.WriteString("\n[git]\ncolocate = true\n")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("config: %v %v", writeErr, closeErr)
	}
	child, err := a.NewWorkspace(t.Context(), "../editor", "@")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(child.Path, ".git")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("child lacks native Git worktree link: %v", err)
	}
	gitOutput := func(root string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if got := gitOutput(child.Path, "rev-parse", "--show-toplevel"); got != child.Path {
		t.Fatalf("editor would use wrong root: %q", got)
	}
	if got := gitOutput(child.Path, "rev-parse", "--path-format=absolute", "--git-common-dir"); got != gitOutput(anchor.Path, "rev-parse", "--path-format=absolute", "--git-common-dir") {
		t.Fatalf("child does not share native Git repository: %q", got)
	}
	if err := os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("editor change"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := gitOutput(child.Path, "status", "--porcelain"); !strings.Contains(got, " M input.txt") && !strings.HasPrefix(got, "M input.txt") {
		t.Fatalf("Git cannot see the workspace edit: %q", got)
	}
}
