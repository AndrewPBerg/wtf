package port

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectBasePort(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected int
	}{
		{
			name:     "next.js project",
			files:    []string{"next.config.js", "package.json"},
			expected: 3000,
		},
		{
			name:     "next.js mjs config",
			files:    []string{"next.config.mjs"},
			expected: 3000,
		},
		{
			name:     "nuxt project",
			files:    []string{"nuxt.config.ts"},
			expected: 3000,
		},
		{
			name:     "astro project",
			files:    []string{"astro.config.mjs"},
			expected: 4321,
		},
		{
			name:     "vite project",
			files:    []string{"vite.config.ts"},
			expected: 5173,
		},
		{
			name:     "svelte project",
			files:    []string{"svelte.config.js"},
			expected: 5173,
		},
		{
			name:     "angular project",
			files:    []string{"angular.json"},
			expected: 4200,
		},
		{
			name:     "django project",
			files:    []string{"manage.py"},
			expected: 8000,
		},
		{
			name:     "go project",
			files:    []string{"go.mod"},
			expected: 8080,
		},
		{
			name:     "rails project",
			files:    []string{"Gemfile"},
			expected: 3000,
		},
		{
			name:     "phoenix project",
			files:    []string{"mix.exs"},
			expected: 4000,
		},
		{
			name:     "no framework detected",
			files:    []string{"README.md"},
			expected: DefaultBasePort,
		},
		{
			name:     "empty directory",
			files:    nil,
			expected: DefaultBasePort,
		},
		{
			name:     "next.js beats vite when both present",
			files:    []string{"next.config.js", "vite.config.ts"},
			expected: 3000,
		},
		{
			name:     "remix project",
			files:    []string{"remix.config.js"},
			expected: 3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte(""), 0o644))
			}
			assert.Equal(t, tt.expected, DetectBasePort(dir))
		})
	}
}

func TestDetectBasePort_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	// Create a directory named "go.mod" — should not match
	require.NoError(t, os.Mkdir(filepath.Join(dir, "go.mod"), 0o755))

	assert.Equal(t, DefaultBasePort, DetectBasePort(dir))
}

func TestDetectFramework_ReturnsNilForUnknown(t *testing.T) {
	dir := t.TempDir()
	assert.Nil(t, DetectFramework(dir))
}

func TestDetectFramework_ReturnsFrameworkWithDevCmd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "next.config.js"), []byte(""), 0o644))

	fw := DetectFramework(dir)
	require.NotNil(t, fw)
	assert.Equal(t, "Next.js", fw.Name)
	assert.Equal(t, 3000, fw.BasePort)
	assert.Contains(t, fw.DevCmd, "next dev")
}

func TestDetectFramework_DjangoDevCmd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manage.py"), []byte(""), 0o644))

	fw := DetectFramework(dir)
	require.NotNil(t, fw)
	assert.Equal(t, "Django", fw.Name)
	assert.Contains(t, fw.DevCmd, "runserver")
}
