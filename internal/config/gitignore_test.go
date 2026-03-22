package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureGitignore_AddsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("*.exe\n"), 0o644))

	modified, err := EnsureGitignore(dir)
	require.NoError(t, err)
	assert.True(t, modified)

	data, err := os.ReadFile(gitignore)
	require.NoError(t, err)
	assert.Contains(t, string(data), ".wt-forge.toml")
}

func TestEnsureGitignore_SkipsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte(".wt-forge.toml\n"), 0o644))

	modified, err := EnsureGitignore(dir)
	require.NoError(t, err)
	assert.False(t, modified)
}

func TestEnsureGitignore_SkipsWhenGlobPatternCovers(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"wt-forge-star", ".wt-forge*\n"},
		{"wt-forge-dot-star", ".wt-forge.*\n"},
		{"star-toml", "*.toml\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			gitignore := filepath.Join(dir, ".gitignore")
			require.NoError(t, os.WriteFile(gitignore, []byte(tt.content), 0o644))

			modified, err := EnsureGitignore(dir)
			require.NoError(t, err)
			assert.False(t, modified)
		})
	}
}

func TestEnsureGitignore_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	modified, err := EnsureGitignore(dir)
	require.NoError(t, err)
	assert.True(t, modified)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".wt-forge.toml")
}

func TestEnsureGitignore_HandlesNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("*.exe"), 0o644))

	modified, err := EnsureGitignore(dir)
	require.NoError(t, err)
	assert.True(t, modified)

	data, err := os.ReadFile(gitignore)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".wt-forge.toml")
	// Should not merge with prior line
	assert.NotContains(t, content, "*.exe.wt-forge")
}

func TestEnsureGitignore_IgnoresComments(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("# .wt-forge.toml\n"), 0o644))

	modified, err := EnsureGitignore(dir)
	require.NoError(t, err)
	assert.True(t, modified, "commented-out entry should not count")
}
