package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerVimMotions(t *testing.T) {
	for _, picker := range []string{"browser", "removal", "review", "empty"} {
		t.Run(picker, func(t *testing.T) {
			rows := []core.WorkspaceView{}
			candidates := []core.RemovalCandidate{}
			for i := 0; i < 23 && picker != "empty"; i++ {
				row := core.WorkspaceView{Workspace: core.Workspace{ID: fmt.Sprint(i), Path: fmt.Sprintf("/repo/%d", i)}, Exists: true}
				rows = append(rows, row)
				candidates = append(candidates, core.RemovalCandidate{WorkspaceView: row, Selectable: true})
			}
			b := newBrowser(rows, "/repo", humanPresentation(&bytes.Buffer{}), false, false)
			r := newRemovalPicker(candidates, "/repo", humanPresentation(&bytes.Buffer{}))
			if picker == "review" {
				for _, row := range candidates {
					r.selected[row.ID] = true
				}
				r.confirming = true
			}
			press := func(key string) int {
				t.Helper()
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
				switch key {
				case "right":
					msg = tea.KeyMsg{Type: tea.KeyRight}
				case "left":
					msg = tea.KeyMsg{Type: tea.KeyLeft}
				case "home":
					msg = tea.KeyMsg{Type: tea.KeyHome}
				case "end":
					msg = tea.KeyMsg{Type: tea.KeyEnd}
				}
				if picker == "browser" || picker == "empty" {
					next, _ := b.Update(msg)
					b = next.(browser)
					return b.cursor
				}
				next, _ := r.Update(msg)
				r = next.(removalPicker)
				return r.cursor
			}
			for _, step := range []struct {
				keys []string
				want int
			}{
				{[]string{"4", "j"}, 4},
				{[]string{"2", "k"}, 2},
				{[]string{"1", "2", "j"}, 14},
				{[]string{"g"}, 14},
				{[]string{"g"}, 10}, // gg stays on page two.
				{[]string{"G"}, 19},
				{[]string{"right"}, 22},
				{[]string{"g", "g"}, 20},
				{[]string{"G"}, 22}, // Partial final page.
				{[]string{"left"}, 12},
				{[]string{"5", "x", "j"}, 13}, // Unrelated keys cancel counts.
				{[]string{"g", "x", "g"}, 13}, // Unrelated keys cancel gg.
				{[]string{"j"}, 14},
				{[]string{"end", "4", "j"}, 3},
				{[]string{"home", "4", "k"}, 19},
				{[]string{"home", "0", "j"}, 1},
				{[]string{"l"}, 11},
				{[]string{"h"}, 1},
				{[]string{"L"}, 11},
				{[]string{"H"}, 1},
			} {
				got := 0
				for _, key := range step.keys {
					got = press(key)
				}
				want := step.want
				if picker == "empty" {
					want = 0
				}
				if got != want {
					t.Fatalf("%v: cursor %d, want %d", step.keys, got, want)
				}
			}
			press("home")
			for range 100 {
				press("9")
			}
			got := press("j")
			if got < 0 || got >= max(1, len(rows)) {
				t.Fatalf("long count produced invalid cursor %d", got)
			}
		})
	}
}

func TestBrowserVimPageEdgesAfterResize(t *testing.T) {
	rows := make([]core.WorkspaceView, 11)
	m := newBrowser(rows, "/repo", humanPresentation(&bytes.Buffer{}), false, false)
	m.cursor = 7
	for _, msg := range []tea.Msg{
		tea.WindowSizeMsg{Width: 100, Height: 10}, // Three rows per page.
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}},
	} {
		next, _ := m.Update(msg)
		m = next.(browser)
	}
	if m.cursor != 8 || !strings.Contains(m.View(), "3/4") {
		t.Fatalf("G ignored resized page: cursor %d", m.cursor)
	}
}
