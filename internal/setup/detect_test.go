package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPackageManager_Priority(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{"pnpm wins over npm", []string{"pnpm-lock.yaml", "package-lock.json"}, "pnpm"},
		{"bun wins over yarn", []string{"bun.lockb", "yarn.lock"}, "bun"},
		{"yarn wins over npm", []string{"yarn.lock", "package-lock.json"}, "yarn"},
		{"npm alone", []string{"package-lock.json"}, "npm"},
		{"uv lock", []string{"uv.lock"}, "uv"},
		{"pyproject.toml fallback", []string{"pyproject.toml"}, "uv"},
		{"pnpm alone", []string{"pnpm-lock.yaml"}, "pnpm"},
		{"bun alone", []string{"bun.lockb"}, "bun"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte(""), 0o644))
			}

			pm, err := DetectPackageManager(dir)
			require.NoError(t, err)
			require.NotNil(t, pm)
			assert.Equal(t, tt.expected, pm.Name)
		})
	}
}

func TestDetectPackageManager_NoMatch(t *testing.T) {
	dir := t.TempDir()

	pm, err := DetectPackageManager(dir)
	assert.NoError(t, err)
	assert.Nil(t, pm)
}

func TestDetectPackageManager_InstallCmd(t *testing.T) {
	tests := []struct {
		lockfile   string
		installCmd string
	}{
		{"pnpm-lock.yaml", "pnpm install"},
		{"bun.lockb", "bun install"},
		{"yarn.lock", "yarn install"},
		{"package-lock.json", "npm install"},
		{"uv.lock", "uv sync"},
	}

	for _, tt := range tests {
		t.Run(tt.lockfile, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.lockfile), []byte(""), 0o644))

			pm, err := DetectPackageManager(dir)
			require.NoError(t, err)
			require.NotNil(t, pm)
			assert.Equal(t, tt.installCmd, pm.InstallCmd)
			assert.Equal(t, tt.lockfile, pm.Lockfile)
		})
	}
}

func TestDetectPackageManager_AdditionalEcosystems(t *testing.T) {
	tests := []struct {
		lockfile   string
		name       string
		installCmd string
	}{
		{"go.sum", "go", "go mod download"},
		{"Cargo.lock", "cargo", "cargo build"},
		{"Gemfile.lock", "bundler", "bundle install"},
		{"composer.lock", "composer", "composer install"},
		{"pom.xml", "maven", "mvn install"},
		{"build.gradle", "gradle", "gradle build"},
		{"build.gradle.kts", "gradle", "gradle build"},
		{"mix.lock", "mix", "mix deps.get"},
		{"Package.resolved", "swift", "swift package resolve"},
		{"packages.lock.json", "dotnet", "dotnet restore"},
		{"poetry.lock", "poetry", "poetry install"},
		{"requirements.txt", "pip", "pip install -r requirements.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.lockfile, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.lockfile), []byte(""), 0o644))

			pm, err := DetectPackageManager(dir)
			require.NoError(t, err)
			require.NotNil(t, pm, "expected detection for %s", tt.lockfile)
			assert.Equal(t, tt.name, pm.Name)
			assert.Equal(t, tt.installCmd, pm.InstallCmd)
		})
	}
}
