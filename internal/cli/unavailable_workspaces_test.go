package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestUnavailableWorkspacesPrunedOnlyWhenConfirmedMissing(t *testing.T) {
	for _, mode := range []string{"missing", "replaced-by-file"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "repo")
			config := filepath.Join(dir, "jj.toml")
			if err := os.WriteFile(config, []byte("[user]\nname='Availability Test'\nemail='availability@example.test'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("JJ_CONFIG", config)
			if output, err := exec.Command("jj", "git", "init", root).CombinedOutput(); err != nil {
				t.Fatalf("jj init: %s %v", output, err)
			}
			a := core.New(root, filepath.Join(dir, "home"), nil)
			if _, err := a.Init(t.Context()); err != nil {
				t.Fatal(err)
			}
			child, err := a.NewWorkspace(t.Context(), "../unavailable-child", "@")
			if err != nil {
				t.Fatal(err)
			}
			backup := child.Path + "-backup"
			if err = os.Rename(child.Path, backup); err != nil {
				t.Fatal(err)
			}
			if mode == "replaced-by-file" {
				if err = os.WriteFile(child.Path, []byte("not a workspace directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			repo, err := core.Discover(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(repo.Store, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{
				{"ls", "-j", "--pr=false", "--checks=false"},
				{"LS", "--ALL", "-J", "--PR=false", "--CHECKS=false"},
				{"rm", "-j"},
			} {
				result, code, _ := invoke(t, a, args...)
				if code != 0 {
					t.Fatalf("%v: %+v", args, result)
				}
				encoded, err := json.Marshal(result.Data)
				if err != nil {
					t.Fatal(err)
				}
				var rows []core.WorkspaceView
				if err = json.Unmarshal(encoded, &rows); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, row := range rows {
					if row.ID == child.ID {
						found = true
						if row.Exists || row.Status != "ready" {
							t.Fatalf("unavailable record lost observation or lifecycle state: %+v", row)
						}
					}
				}
				if found != (args[0] == "LS" && mode == "replaced-by-file") {
					t.Fatalf("%v: unavailable visibility=%v", args, found)
				}
			}
			var out, logs bytes.Buffer
			session := New(a, &out, &logs)
			if code := session.Execute(context.Background(), []string{"ls", "--pr=false", "--checks=false"}); code != 0 || strings.Contains(out.String(), child.Name) {
				t.Fatalf("human list includes unavailable workspace: %s %s", out.String(), logs.String())
			}
			rows, err := a.List(t.Context(), false, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range removalPreview(rows, root) {
				if candidate.ID == child.ID {
					t.Fatal("removal picker includes unavailable workspace")
				}
			}
			result, code, _ := invoke(t, a, "ls", child.Name, "-j")
			wantCode := "workspace_unavailable"
			if mode == "missing" {
				wantCode = "workspace_not_found"
			}
			if code == 0 || result.Error == nil || result.Error.Code != wantCode {
				t.Fatalf("explicit unavailable selector: %+v", result)
			}
			after, err := os.ReadFile(filepath.Join(repo.Store, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(before, after) != (mode == "replaced-by-file") {
				t.Fatal("registry reconciliation did not distinguish absence from an unavailable directory")
			}
			if mode == "missing" {
				replacement, err := a.NewWorkspace(t.Context(), child.Path, "@")
				if err != nil || replacement.ID == child.ID {
					t.Fatalf("stale name could not be reused: %+v %v", replacement, err)
				}
				if _, err = os.Stat(backup); err != nil {
					t.Fatal("listing touched moved workspace files", err)
				}
				return
			}
			if mode == "replaced-by-file" {
				if err = os.Remove(child.Path); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.Rename(backup, child.Path); err != nil {
				t.Fatal(err)
			}
			result, code, _ = invoke(t, a, "ls", child.Name, "-j")
			if code != 0 || !result.OK {
				t.Fatalf("restored workspace did not reappear: %+v", result)
			}
		})
	}
}
