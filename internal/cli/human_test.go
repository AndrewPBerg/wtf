package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/lipgloss"
)

func themeFixture(t *testing.T) string {
	t.Helper()
	state, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_CONFIG_HOME", config)
	dir := filepath.Join(state, "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "colors.toml")
	if err := os.WriteFile(path, []byte("accent='#bb9af7'\nforeground='#ffffff'\ncolor8='#6b578f'\ncolor2='#c2b8ff'\ncolor3='#ddccff'\ncolor1='#f07178'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHumanListIsReadableAndKeepsMetadataInJSONOnly(t *testing.T) {
	themeFixture(t)
	t.Setenv("NO_COLOR", "1")
	var out, logs bytes.Buffer
	s := New(core.New("/repo/auth", t.TempDir(), &logs), &out, &logs)
	s.Command = "wtf list"
	s.Data = []core.WorkspaceView{
		{Workspace: core.Workspace{ID: "private-uuid", Name: "auth", Path: "/repo/auth", Status: "ready", Stages: map[string]core.Stage{"prepare": {Status: "succeeded"}}}, Exists: true},
		{Workspace: core.Workspace{ID: "other-uuid", Name: "experiment", Path: "/repo/experiment", Status: "hook_failed", Stages: map[string]core.Stage{"test": {Status: "failed"}}}, Exists: true},
	}
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Workspaces", "WORKSPACE", "AGE", "auth", "experiment", "●"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	for _, unwanted := range []string{"PATH", "../experiment", "LOCAL", "✓", "hook failed", "current workspace", "full records", "CHECKS", "STAGES", "1/1", "private-uuid", "fingerprint", "repository_id", "\x1b[", "\"ok\""} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("human output leaked %q: %s", unwanted, text)
		}
	}
	if json.Valid(out.Bytes()) {
		t.Fatal("default output is still JSON")
	}
	out.Reset()
	s.ShowStages = true
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"STAGES", "1/1", "0/1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("opt-in stages missing %q: %s", want, out.String())
		}
	}
}

func TestThemeChangesApplyOnNextInvocationAndJSONStaysUnstyled(t *testing.T) {
	path := themeFixture(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("FORCE_COLOR", "1")
	var out, logs bytes.Buffer
	a := core.New(t.TempDir(), t.TempDir(), &logs)
	s := New(a, &out, &logs)
	s.Data = map[string]string{"version": "test"}
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "38;2;187;154;247") {
		t.Fatalf("active accent not used: %q", out.String())
	}
	if err := os.WriteFile(path, []byte("accent='#123456'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	s = New(a, &out, &logs)
	s.Data = map[string]string{"version": "test"}
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "38;2;18;52;86") {
		t.Fatalf("new accent not loaded: %q", out.String())
	}
	out.Reset()
	s = New(a, &out, &logs)
	s.JSON = true
	s.Command = "wtf version"
	s.Data = map[string]string{"version": "test"}
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) || strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("JSON changed: %q", out.String())
	}
}

func TestNoColorOverridesForcedColorAndInvalidPaletteFallsBack(t *testing.T) {
	path := themeFixture(t)
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	if err := os.WriteFile(path, []byte("accent='not-a-color'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if p := activePalette(); p.Accent != "4" {
		t.Fatalf("bad palette did not fall back: %+v", p)
	}
	var out, logs bytes.Buffer
	s := New(core.New(t.TempDir(), t.TempDir(), &logs), &out, &logs)
	s.Data = map[string]string{"version": "test"}
	if err := s.emit(nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("NO_COLOR ignored: %q", out.String())
	}
	if plain("workspace\x1b[31m\nnext") != "workspace [31m next" {
		t.Fatal("terminal controls were not sanitized")
	}
}

func TestWorkspaceTableKeepsBothBordersInsideTerminalWidth(t *testing.T) {
	themeFixture(t)
	t.Setenv("NO_COLOR", "1")
	var out bytes.Buffer
	rows := []core.WorkspaceView{
		{Workspace: core.Workspace{Name: "a-very-long-workspace-name-that-must-not-expand-the-table", Path: "/repo/very-long-directory/子目录/another-long-directory/workspace", Status: "cleanup_failed"}, Exists: true},
		{Workspace: core.Workspace{Name: "current", Path: "/repo/current", Status: "ready"}, Exists: true},
	}
	for _, showStages := range []bool{false, true} {
		for _, showPRs := range []bool{false, true} {
			for _, width := range []int{34, 60, 71, 80, 100, 200} {
				p := humanPresentation(&out)
				p.Width = width
				text := workspaceTable(p, rows, "/repo/current", showStages, showPRs)
				lines := strings.Split(text, "\n")
				for _, line := range lines {
					if lipgloss.Width(line) > width-1 {
						t.Fatalf("width %d clipped: %q (%d cells)", width, line, lipgloss.Width(line))
					}
				}
				if strings.Contains(text, "╭") {
					if !strings.HasSuffix(lines[1], "╮") || !strings.HasSuffix(lines[len(lines)-1], "╯") {
						t.Fatalf("width %d lost the right border:\n%s", width, text)
					}
				}
			}
		}
	}
}
