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
