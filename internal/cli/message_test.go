package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestNewMessageFlagRoundTripDefaultsAndCase(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	cfg := filepath.Join(dir, "jj.toml")
	if err := os.WriteFile(cfg, []byte("[user]\nname='Message Test'\nemail='message@example.test'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JJ_CONFIG", cfg)
	if data, err := exec.Command("jj", "git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("init: %s %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".wtf/workspace.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := core.New(root, filepath.Join(dir, "home"), nil)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"NeW", ".", "--MeSsAgE", "Anchor Description"}, "Anchor Description"},
		{[]string{"NEW", "ChildCase", "-M", "Hello This Is Testing 日本", "-F", "@"}, "Hello This Is Testing 日本"},
		{[]string{"new", "ChildCase"}, "Hello This Is Testing 日本"},
		{[]string{"new", "ChildCase", "-m", "Updated"}, "Updated"},
	} {
		result, code, logs := invoke(t, a, append(test.args, "-J")...)
		if code != 0 {
			t.Fatalf("%v: %+v %s", test.args, result, logs)
		}
		data, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		var workspace core.Initialization
		if err := json.Unmarshal(data, &workspace); err != nil || workspace.Message != test.want {
			t.Fatalf("message changed: %s %v", data, err)
		}
		if workspace.Name == "ChildCase" && workspace.Path != filepath.Join(dir, "ChildCase") {
			t.Fatalf("bare name did not create a sibling: %+v", workspace)
		}
	}
	for _, args := range [][]string{{"LS", "--pr=false", "--checks=false"}, {"RM"}} {
		result, code, logs := invoke(t, a, append(args, "-J")...)
		if code != 0 {
			t.Fatalf("%v: %+v %s", args, result, logs)
		}
		data, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		var rows []core.WorkspaceView
		if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 2 {
			t.Fatalf("missing metadata: %s %v", data, err)
		}
		for _, row := range rows {
			want := "Anchor Description"
			if row.Name == "ChildCase" {
				want = "Updated"
			}
			if row.Message != want || !filepath.IsAbs(row.Path) {
				t.Fatalf("message or path missing: %+v", row)
			}
		}
	}
	writeDefaults(t, filepath.Join(root, ".wtf", "config.toml"), "[commands.new]\nmessage='Inherited Description'\n")
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "ChildCase"}, "Inherited Description"},
		{[]string{"new", "ChildCase", "--MESSAGE", ""}, ""},
	} {
		result, code, logs := invoke(t, a, append(test.args, "-j")...)
		if code != 0 {
			t.Fatalf("defaults: %+v %s", result, logs)
		}
		data, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		var workspace core.Workspace
		if err := json.Unmarshal(data, &workspace); err != nil || workspace.Message != test.want {
			t.Fatalf("message precedence changed: %s %v", data, err)
		}
	}
}

func TestWorkspaceMessagesFollowSelectionWithoutReplacingRemovalWarnings(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "one", Name: "one", Path: "/repo/one", Message: "First description"}, Exists: true},
		{Workspace: core.Workspace{ID: "two", Name: "two", Path: "/repo/two", Message: "Second description"}, Exists: true},
		{Workspace: core.Workspace{ID: "empty", Name: "empty", Path: "/repo/empty"}, Exists: true},
	}
	p := humanPresentation(&bytes.Buffer{})
	p.Width = 120
	list := newBrowser(rows, "/repo/one", p, false, false)
	remove := newRemovalPicker([]core.RemovalCandidate{
		{WorkspaceView: rows[0], Warning: "Changes remain; preserve them", Code: "workspace_has_changes"},
		{WorkspaceView: rows[1], Selectable: true},
		{WorkspaceView: rows[2], Selectable: true},
	}, "/repo/one", p)
	if view := list.View(); !strings.Contains(view, "First description") || strings.Contains(view, "Second description") {
		t.Fatalf("list description does not follow selection:\n%s", view)
	}
	if view := remove.View(); !strings.Contains(view, "First description") || !strings.Contains(view, "Changes remain; preserve them") {
		t.Fatalf("removal description hid a warning:\n%s", view)
	}
	next, _ := list.Update(tea.KeyMsg{Type: tea.KeyDown})
	list = next.(browser)
	remove.cursor = 1
	for _, view := range []string{list.View(), remove.View()} {
		if !strings.Contains(view, "Second description") || strings.Contains(view, "First description") || strings.Contains(view, "PATH") {
			t.Fatalf("selection did not update the description:\n%s", view)
		}
	}
	list.cursor, remove.cursor = 2, 2
	for _, view := range []string{list.View(), remove.View()} {
		if strings.Contains(view, "Message:") || strings.Contains(view, "description") {
			t.Fatalf("empty workspace retained another message:\n%s", view)
		}
	}
	long := strings.Repeat("説明", 100) + "\n\x1b[31m"
	for i := range rows {
		rows[i].Message = long
	}
	list.setRows(rows)
	for i := range remove.rows {
		remove.rows[i].Message = long
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 12}, {Width: 34, Height: 10}, {Width: 9, Height: 5}} {
		next, _ := list.Update(size)
		list = next.(browser)
		next, _ = remove.Update(size)
		remove = next.(removalPicker)
		for _, view := range []string{list.View(), remove.View()} {
			if lipgloss.Height(view) > size.Height || strings.Contains(view, "\x1b[31m") {
				t.Fatalf("message expanded height or emitted controls:\n%s", view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.Width-1 {
					t.Fatalf("message expanded width: %q", line)
				}
			}
		}
	}
}
