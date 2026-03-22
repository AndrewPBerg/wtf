package notify

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDesktop_LinuxNotifySend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}

	cfg := &config{
		lookPath: func(name string) (string, error) {
			if name == "notify-send" {
				return "/usr/bin/notify-send", nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	d := newDesktop(cfg)
	require.NotNil(t, d)
	assert.Equal(t, "notify-send", d.cmd)
}

func TestNewDesktop_DarwinOsascript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}

	cfg := &config{
		lookPath: func(name string) (string, error) {
			if name == "osascript" {
				return "/usr/bin/osascript", nil
			}
			return "", fmt.Errorf("not found")
		},
	}

	d := newDesktop(cfg)
	require.NotNil(t, d)
	assert.Equal(t, "osascript", d.cmd)
}

func TestNewDesktop_NothingFound(t *testing.T) {
	cfg := &config{
		lookPath: func(_ string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	d := newDesktop(cfg)
	assert.Nil(t, d)
}

func TestDesktopNotifier_Name(t *testing.T) {
	d := &desktopNotifier{cmd: "notify-send"}
	assert.Equal(t, "desktop", d.Name())
}

func TestDesktopNotifier_Notify_NotifySend(t *testing.T) {
	var capturedArgs []string
	d := &desktopNotifier{
		cmd: "notify-send",
		run: func(_ context.Context, name string, args ...string) error {
			capturedArgs = append([]string{name}, args...)
			return nil
		},
	}

	err := d.Notify(context.Background(), "WTF", "PR #42 approved")
	require.NoError(t, err)
	assert.Equal(t, []string{"notify-send", "-a", "wtf", "WTF", "PR #42 approved"}, capturedArgs)
}

func TestDesktopNotifier_Notify_Osascript(t *testing.T) {
	var capturedArgs []string
	d := &desktopNotifier{
		cmd: "osascript",
		run: func(_ context.Context, name string, args ...string) error {
			capturedArgs = append([]string{name}, args...)
			return nil
		},
	}

	err := d.Notify(context.Background(), "WTF", "PR #42 approved")
	require.NoError(t, err)
	assert.Equal(t, "osascript", capturedArgs[0])
	assert.Equal(t, "-e", capturedArgs[1])
	assert.Contains(t, capturedArgs[2], "display notification")
	assert.Contains(t, capturedArgs[2], "PR #42 approved")
}

func TestDesktopNotifier_Notify_UnknownCmd(t *testing.T) {
	d := &desktopNotifier{
		cmd: "unknown",
		run: func(_ context.Context, _ string, _ ...string) error { return nil },
	}

	err := d.Notify(context.Background(), "WTF", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown desktop command")
}

func TestDesktopNotifier_Notify_RunError(t *testing.T) {
	d := &desktopNotifier{
		cmd: "notify-send",
		run: func(_ context.Context, _ string, _ ...string) error {
			return fmt.Errorf("command failed")
		},
	}

	err := d.Notify(context.Background(), "WTF", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sending desktop notification")
}

func TestNewDesktop_WithCmdRunner(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}

	called := false
	cfg := &config{
		lookPath: func(name string) (string, error) {
			if name == "notify-send" {
				return "/usr/bin/notify-send", nil
			}
			return "", fmt.Errorf("not found")
		},
		cmdRunner: func(_ context.Context, _ string, _ ...string) error {
			called = true
			return nil
		},
	}

	d := newDesktop(cfg)
	require.NotNil(t, d)

	err := d.Notify(context.Background(), "Test", "body")
	require.NoError(t, err)
	assert.True(t, called)
}

func TestDesktopNotifier_Notify_OsascriptError(t *testing.T) {
	d := &desktopNotifier{
		cmd: "osascript",
		run: func(_ context.Context, _ string, _ ...string) error {
			return fmt.Errorf("osascript failed")
		},
	}

	err := d.Notify(context.Background(), "WTF", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sending desktop notification")
}
