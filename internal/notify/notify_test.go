package notify

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_TerminalOnly(t *testing.T) {
	n := New(WithTerminalOnly(true))
	assert.Equal(t, "terminal", n.Name())
}

func TestNew_DesktopFound(t *testing.T) {
	n := New(withLookPath(func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}))
	assert.Equal(t, "desktop", n.Name())
}

func TestNew_DesktopNotFound_FallsBackToTerminal(t *testing.T) {
	n := New(withLookPath(func(_ string) (string, error) {
		return "", fmt.Errorf("not found")
	}))
	assert.Equal(t, "terminal", n.Name())
}

func TestNew_CustomWriter(t *testing.T) {
	var buf bytes.Buffer
	n := New(WithTerminalOnly(true), WithWriter(&buf))

	err := n.Notify(context.Background(), "Test", "hello")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "hello")
}

func TestNew_WithCmdRunner(t *testing.T) {
	called := false
	n := New(
		withLookPath(func(_ string) (string, error) {
			return "/usr/bin/notify-send", nil
		}),
		withCmdRunner(func(_ context.Context, _ string, _ ...string) error {
			called = true
			return nil
		}),
	)

	// On Linux this will be desktop, on other platforms terminal.
	// Either way, calling Notify should work.
	err := n.Notify(context.Background(), "Test", "body")
	require.NoError(t, err)

	if n.Name() == "desktop" {
		assert.True(t, called)
	}
}
