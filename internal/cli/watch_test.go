package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/notify"
	"github.com/AndrewPBerg/wtf/internal/watch"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestResolveStateDir(t *testing.T) {
	dir := initCLITestRepo(t)

	exec := &git.RealExecutor{}
	stateDir, err := resolveStateDir(exec, dir)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(stateDir, "/wtf") || strings.HasSuffix(stateDir, string(os.PathSeparator)+"wtf"))
	assert.Contains(t, stateDir, ".git")
}

func TestPrintBanner(t *testing.T) {
	stderr := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetErr(stderr)

	n := notify.New(notify.WithTerminalOnly(true))
	printBanner(cmd, "my-repo", 60*time.Second, n)

	output := stderr.String()
	assert.Contains(t, output, "my-repo")
	assert.Contains(t, output, "1m0s")
	assert.Contains(t, output, "Terminal only")
}

func TestPrintBanner_Desktop(t *testing.T) {
	stderr := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetErr(stderr)

	n := notify.New()
	printBanner(cmd, "test-repo", 30*time.Second, n)

	output := stderr.String()
	assert.Contains(t, output, "test-repo")
	assert.Contains(t, output, "30s")
	// Desktop notifier name contains "desktop"
	if strings.Contains(n.Name(), "desktop") {
		assert.Contains(t, output, "Desktop + terminal")
	}
}

func TestResolveInterval_FromConfig(t *testing.T) {
	watchInterval = 0
	defer func() { watchInterval = 0 }()

	dir := t.TempDir()
	// Create a .wt-forge.toml with watch interval
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wt-forge.toml"), []byte("[watch]\ninterval = 120\n"), 0o644))

	d := resolveInterval(dir)
	assert.Equal(t, 120*time.Second, d)
}

func TestResolveInterval_ConfigClampedToMin(t *testing.T) {
	watchInterval = 0
	defer func() { watchInterval = 0 }()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wt-forge.toml"), []byte("[watch]\ninterval = 3\n"), 0o644))

	d := resolveInterval(dir)
	assert.Equal(t, minInterval, d)
}

func TestResolveNotifier_FromConfig(t *testing.T) {
	watchNoDesktop = false
	defer func() { watchNoDesktop = false }()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".wt-forge.toml"), []byte("[watch]\ndesktop = false\n"), 0o644))

	n := resolveNotifier(dir)
	assert.Equal(t, "terminal", n.Name())
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
