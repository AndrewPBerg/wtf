package core

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNotifyPlainFallbackSanitizesMessagesAndValidatesLevels(t *testing.T) {
	var logs bytes.Buffer
	a := New(t.TempDir(), t.TempDir(), &logs)
	if err := a.Notify("\x1b[31mhello\x1b[0m\nworld", "SUCCESS"); err != nil {
		t.Fatal(err)
	}
	if logs.String() != "[success] hello world\n" {
		t.Fatalf("unsafe or styled fallback: %q", logs.String())
	}
	logs.Reset()
	if err := a.Notify("hello", ""); err != nil || logs.String() != "[info] hello\n" {
		t.Fatalf("default level: %q %v", logs.String(), err)
	}
	logs.Reset()
	if err := a.Notify("hello", "unknown"); ErrorCode(err) != "invalid_notification" || logs.Len() != 0 {
		t.Fatalf("invalid level was printed: %q %v", logs.String(), err)
	}
	writeErr := errors.New("notification sink unavailable")
	a.Notifier = func(Notification) error { return writeErr }
	if err := a.Notify("hello", "info"); !errors.Is(err, writeErr) {
		t.Fatalf("lost sink failure: %v", err)
	}
}

func TestLuaNotifyUsesHostSinkAndTypedErrors(t *testing.T) {
	a, _, w := fixture(t)
	var received []Notification
	a.Notifier = func(note Notification) error {
		received = append(received, note)
		return nil
	}
	creationHookFiles(t, w.Path, []string{"hello"}, map[string]string{"hello.lua": `local wtf = require("wtf")
return function(event)
  wtf.notify("hello")
  wtf.notify("success", "success")
  wtf.notify("warning", "warning")
  wtf.notify("error", "error")
  local ok, err = pcall(function() wtf.notify("bad", "unknown") end)
  assert(not ok and err.code == "invalid_notification")
end`})
	if _, err := a.Hook(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if len(received) != 4 || received[0].Level != "info" || received[1].Level != "success" || received[2].Level != "warning" || received[3].Level != "error" {
		t.Fatalf("host sink did not receive levels: %+v", received)
	}
	a.Notifier = func(Notification) error { return Fail("notification_failed", "Host output unavailable") }
	if _, err := a.Hook(context.Background(), "hello"); ErrorCode(err) != "notification_failed" || !strings.Contains(err.Error(), "Host output unavailable") {
		t.Fatalf("Lua lost typed sink failure: %v", err)
	}
}
