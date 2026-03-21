package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUninstallCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "uninstall" {
			found = true
			break
		}
	}
	assert.True(t, found, "uninstall command should be registered")
}

func TestUninstallCommandMetadata(t *testing.T) {
	assert.Equal(t, "uninstall", uninstallCmd.Use)
	assert.Contains(t, uninstallCmd.Short, "Remove")
}

func TestFindBinary_NotInPath(t *testing.T) {
	// Use an empty PATH so wtf won't be found
	orig := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer func() { _ = os.Setenv("PATH", orig) }()

	_, err := findBinary()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindBinary_Found(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wtf")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer func() { _ = os.Setenv("PATH", orig) }()

	path, err := findBinary()
	assert.NoError(t, err)
	assert.Equal(t, bin, path)
}

func TestRunUninstall_ForceRemovesBinary(t *testing.T) {
	// Create a fake wtf binary
	dir := t.TempDir()
	bin := filepath.Join(dir, "wtf")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	t.Setenv("PATH", dir)

	uninstallForce = true
	defer func() { uninstallForce = false }()

	buf := new(bytes.Buffer)
	cmd := uninstallCmd
	cmd.SetOut(buf)

	err := runUninstall(cmd)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Removed")
	_, statErr := os.Stat(bin)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRunUninstall_ConfirmAccepted(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wtf")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	t.Setenv("PATH", dir)

	uninstallForce = false
	defer func() { uninstallForce = false }()

	buf := new(bytes.Buffer)
	cmd := uninstallCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := runUninstall(cmd)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Removed")
	_, statErr := os.Stat(bin)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRunUninstall_ConfirmDeclined(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wtf")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	t.Setenv("PATH", dir)

	uninstallForce = false
	defer func() { uninstallForce = false }()

	buf := new(bytes.Buffer)
	cmd := uninstallCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := runUninstall(cmd)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Aborted")
	// Binary should still exist
	_, statErr := os.Stat(bin)
	assert.NoError(t, statErr)
}

func TestRunUninstall_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	uninstallForce = true
	defer func() { uninstallForce = false }()

	buf := new(bytes.Buffer)
	cmd := uninstallCmd
	cmd.SetOut(buf)

	err := runUninstall(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunUninstall_RemovePermissionError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wtf")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	// Make directory read-only to prevent removal
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	t.Setenv("PATH", dir)

	uninstallForce = true
	defer func() { uninstallForce = false }()

	buf := new(bytes.Buffer)
	cmd := uninstallCmd
	cmd.SetOut(buf)

	err := runUninstall(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "removing")
}

func TestConfirmPrompt(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"y confirms", "y\n", true},
		{"yes confirms", "yes\n", true},
		{"Y confirms", "Y\n", true},
		{"YES confirms", "YES\n", true},
		{"n declines", "n\n", false},
		{"empty declines", "\n", false},
		{"random declines", "maybe\n", false},
		{"eof declines", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := new(bytes.Buffer)
			result := confirmPrompt(in, out, "Continue? [y/N] ")
			assert.Equal(t, tt.expect, result)
			assert.Contains(t, out.String(), "Continue?")
		})
	}
}
