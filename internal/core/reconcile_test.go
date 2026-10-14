package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReconcileMissingWorkspacesPreservesOwnersAndBusyOperations(t *testing.T) {
	for _, mode := range []string{"plain", "claimed", "creating", "locked", "git-locked", "broken-symlink", "native-reassigned"} {
		t.Run(mode, func(t *testing.T) {
			a, r, _ := fixture(t)
			child, err := a.NewWorkspace(t.Context(), "../missing", "@")
			if err != nil {
				t.Fatal(err)
			}
			backup := child.Path + "-backup"
			if err = os.Rename(child.Path, backup); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "claimed":
				if _, err = r.Claim(t.Context(), child.Workspace, "port", "web", "3000"); err != nil {
					t.Fatal(err)
				}
			case "creating":
				if err = r.Update(t.Context(), child.ID, func(w *Workspace) { w.Status = "creating" }); err != nil {
					t.Fatal(err)
				}
			case "locked":
				unlock, lockErr := r.LockWorkspace(t.Context(), child.ID)
				if lockErr != nil {
					t.Fatal(lockErr)
				}
				defer unlock()
			case "broken-symlink":
				if err = os.Symlink(filepath.Join(backup, "absent"), child.Path); err != nil {
					t.Fatal(err)
				}
			case "git-locked":
				command := exec.Command("git", "worktree", "lock", child.Path)
				command.Dir = r.Root
				if data, lockErr := command.CombinedOutput(); lockErr != nil {
					t.Fatalf("git worktree lock: %s %v", data, lockErr)
				}
			case "native-reassigned":
				if _, err = output(t.Context(), r.Root, "workspace", "forget", "--ignore-working-copy", child.Name); err != nil {
					t.Fatal(err)
				}
				if _, err = output(t.Context(), r.Root, "workspace", "add", "--name", child.Name, "-r", "@", filepath.Join(filepath.Dir(child.Path), "replacement")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = a.List(t.Context(), false, false); err != nil {
				t.Fatal(err)
			}
			state, err := r.Load()
			if err != nil {
				t.Fatal(err)
			}
			_, registered := state.Workspaces[child.ID]
			pruned := mode == "plain" || mode == "native-reassigned"
			if registered == pruned {
				t.Fatalf("mode=%s registered=%v", mode, registered)
			}
			native, err := NativeWorkspaces(t.Context(), r.Root)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := native[child.Name]; exists != (mode != "plain") {
				t.Fatalf("wrong native registration after %s", mode)
			}
			if mode == "claimed" && len(state.Claims) != 1 {
				t.Fatal("resource ownership lost")
			}
			if _, err = os.Stat(backup); err != nil {
				t.Fatal("workspace files changed", err)
			}
		})
	}
}
