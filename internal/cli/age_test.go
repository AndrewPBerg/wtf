package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/lipgloss"
)

func TestWorkspaceAgeUnitsAndBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		age  time.Duration
		want string
	}{
		{-time.Minute, "0S"}, {0, "0S"}, {time.Second / 2, "0S"},
		{59 * time.Second, "59S"}, {time.Minute, "1M"},
		{time.Hour - time.Second, "59M"}, {time.Hour, "1H"},
		{24*time.Hour - time.Second, "23H"}, {24 * time.Hour, "1D"},
		{7*24*time.Hour - time.Second, "6D"}, {7 * 24 * time.Hour, "1W"},
		{10 * 24 * time.Hour, "1W"}, {1000 * 7 * 24 * time.Hour, "1000W"},
	} {
		created := now.Add(-test.age)
		if got := workspaceAge(&created, now); got != test.want {
			t.Errorf("age %s: got %q, want %q", test.age, got, test.want)
		}
	}
	zero := time.Time{}
	for _, unknown := range []*time.Time{nil, &zero} {
		if got := workspaceAge(unknown, now); got != "?" {
			t.Fatalf("unknown age: %q", got)
		}
	}
}

func TestAgeVisibleInListAndRemovalViews(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	created := time.Now().Add(-50 * time.Hour)
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "known", Name: "known", Path: "/repo/known", CreatedAt: &created}, Exists: true},
		{Workspace: core.Workspace{ID: "legacy", Name: "legacy", Path: "/repo/legacy"}, Exists: true},
	}
	p := humanPresentation(&bytes.Buffer{})
	p.Width = 120
	list := newBrowser(rows, "/repo", p, false, false)
	remove := newRemovalPicker([]core.RemovalCandidate{
		{WorkspaceView: rows[0], Integrated: true, Selectable: true},
		{WorkspaceView: rows[1], Integrated: true, Selectable: true},
	}, "/repo", p)
	views := map[string]string{
		"list table":    workspaceTable(p, rows, "/repo", false, false),
		"list picker":   list.View(),
		"remove picker": remove.View(),
	}
	remove.selected["known"], remove.selected["legacy"] = true, true
	remove.confirming = true
	views["remove review"] = remove.View()
	for name, view := range views {
		for _, want := range []string{"AGE", "2D", "?", "known", "legacy"} {
			if !strings.Contains(view, want) {
				t.Errorf("%s missing %q:\n%s", name, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > p.Width-1 {
				t.Fatalf("%s exceeds terminal width: %q", name, line)
			}
		}
	}
	p.Width = 34
	if view := workspaceTable(p, rows, "/repo", false, false); !strings.Contains(view, "age 2D") || !strings.Contains(view, "age ?") {
		t.Fatalf("narrow list lost age:\n%s", view)
	}
}
