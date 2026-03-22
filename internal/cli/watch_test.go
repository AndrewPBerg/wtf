package cli

import (
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/notify"
	"github.com/AndrewPBerg/wtf/internal/watch"
	"github.com/stretchr/testify/assert"
)

func TestResolveInterval_Default(t *testing.T) {
	watchInterval = 0
	defer func() { watchInterval = 0 }()

	d := resolveInterval("")
	assert.Equal(t, watch.DefaultInterval, d)
}

func TestResolveInterval_Flag(t *testing.T) {
	watchInterval = 30
	defer func() { watchInterval = 0 }()

	d := resolveInterval("")
	assert.Equal(t, 30*time.Second, d)
}

func TestResolveInterval_ClampedToMinimum(t *testing.T) {
	watchInterval = 3
	defer func() { watchInterval = 0 }()

	d := resolveInterval("")
	assert.Equal(t, minInterval, d)
}

func TestClampInterval(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"below minimum", 5 * time.Second, minInterval},
		{"at minimum", minInterval, minInterval},
		{"above minimum", 120 * time.Second, 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clampInterval(tt.in))
		})
	}
}

func TestResolveNotifier_Default(t *testing.T) {
	watchNoDesktop = false
	defer func() { watchNoDesktop = false }()

	n := resolveNotifier("")
	assert.NotNil(t, n)
}

func TestResolveNotifier_NoDesktop(t *testing.T) {
	watchNoDesktop = true
	defer func() { watchNoDesktop = false }()

	n := resolveNotifier("")
	assert.Equal(t, "terminal", n.Name())
}

func TestResolveNotifier_TerminalOnly(t *testing.T) {
	n := notify.New(notify.WithTerminalOnly(true))
	assert.Equal(t, "terminal", n.Name())
}

func TestWatchCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "watch" {
			found = true
			break
		}
	}
	assert.True(t, found, "watch command should be registered")
}

func TestWatchCmd_Flags(t *testing.T) {
	f := watchCmd.Flags()

	flag := f.Lookup("global")
	assert.NotNil(t, flag)
	assert.Equal(t, "g", flag.Shorthand)

	flag = f.Lookup("interval")
	assert.NotNil(t, flag)
	assert.Equal(t, "i", flag.Shorthand)

	flag = f.Lookup("no-desktop")
	assert.NotNil(t, flag)
}
