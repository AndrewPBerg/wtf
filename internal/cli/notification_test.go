package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestNotificationUsesActivePaletteForEachLevel(t *testing.T) {
	path := themeFixture(t)
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	var out bytes.Buffer
	for _, tc := range []struct{ level, color, prefix string }{
		{"info", "38;2;187;154;247", "•"},
		{"success", "38;2;194;184;255", "✓"},
		{"warning", "38;2;221;204;255", "!"},
		{"error", "38;2;240;113;120", "✗"},
	} {
		text := notificationText(humanPresentation(&out), core.Notification{Message: "hello world", Level: tc.level})
		if !strings.Contains(text, tc.color) || !strings.Contains(text, tc.prefix) || !strings.Contains(text, "hello world") {
			t.Fatalf("%s ignored palette: %q", tc.level, text)
		}
	}
	if err := os.WriteFile(path, []byte("color2='#123456'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text := notificationText(humanPresentation(&out), core.Notification{Message: "hello", Level: "success"})
	if !strings.Contains(text, "38;2;18;52;86") {
		t.Fatalf("notification did not pick up new theme: %q", text)
	}
	t.Setenv("NO_COLOR", "1")
	text = notificationText(humanPresentation(&out), core.Notification{Message: "hello", Level: "success"})
	if strings.Contains(text, "\x1b") {
		t.Fatalf("notification ignored NO_COLOR: %q", text)
	}
}

func TestNotificationLogsStayPlainForJSONAndRedirectedOutput(t *testing.T) {
	themeFixture(t)
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	for _, jsonMode := range []bool{false, true} {
		var out, logs bytes.Buffer
		a := core.New(t.TempDir(), t.TempDir(), &logs)
		s := New(a, &out, &logs)
		s.JSON = jsonMode
		if err := a.Notify("hello world", "success"); err != nil {
			t.Fatal(err)
		}
		if out.Len() != 0 || logs.String() != "✓ hello world\n" {
			t.Fatalf("JSON=%v notification changed stdout or was styled: out=%q logs=%q", jsonMode, out.String(), logs.String())
		}
	}
}
