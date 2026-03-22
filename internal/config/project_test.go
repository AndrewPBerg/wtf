package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProjectConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `
[worktree]
root_path = "/code/trees"
default_base = "develop"

[env]
strategy = "symlink"
files = [".env", ".env.local"]

[[setup]]
name = "install"
run = "pnpm install"

[[setup]]
name = "migrate"
run = "pnpm db:migrate"
if = "file exists 'prisma/schema.prisma'"

[hooks]
on_create = ["echo created"]
on_switch = ["echo switched"]
on_remove = ["echo removed"]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte(content), 0o644))

	cfg, err := LoadProjectConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "/code/trees", cfg.Worktree.RootPath)
	assert.Equal(t, "develop", cfg.Worktree.DefaultBase)
	assert.Equal(t, "symlink", cfg.Env.Strategy)
	assert.Equal(t, []string{".env", ".env.local"}, cfg.Env.Files)
	assert.Len(t, cfg.Setup, 2)
	assert.Equal(t, "install", cfg.Setup[0].Name)
	assert.Equal(t, "pnpm install", cfg.Setup[0].Run)
	assert.Equal(t, "migrate", cfg.Setup[1].Name)
	assert.Equal(t, "file exists 'prisma/schema.prisma'", cfg.Setup[1].If)
	assert.Equal(t, []string{"echo created"}, cfg.Hooks.OnCreate)
	assert.Equal(t, []string{"echo switched"}, cfg.Hooks.OnSwitch)
	assert.Equal(t, []string{"echo removed"}, cfg.Hooks.OnRemove)
}

func TestLoadProjectConfig_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	content := `
[env]
strategy = "copy"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte(content), 0o644))

	cfg, err := LoadProjectConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "copy", cfg.Env.Strategy)
	assert.Empty(t, cfg.Worktree.RootPath)
	assert.Empty(t, cfg.Setup)
	assert.Empty(t, cfg.Hooks.OnCreate)
}

func TestLoadProjectConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadProjectConfig(dir)
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadProjectConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte("not valid [[ toml"), 0o644))

	cfg, err := LoadProjectConfig(dir)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "parsing project config")
}

func TestLoadProjectConfig_ReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectConfigFile)
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	cfg, err := LoadProjectConfig(dir)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "reading project config")
}

func TestValidateProjectConfig_Valid(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ProjectConfig
	}{
		{"nil config", nil},
		{"empty config", &ProjectConfig{}},
		{"symlink strategy", &ProjectConfig{Env: EnvConfig{Strategy: "symlink"}}},
		{"copy strategy", &ProjectConfig{Env: EnvConfig{Strategy: "copy"}}},
		{"none strategy", &ProjectConfig{Env: EnvConfig{Strategy: "none"}}},
		{"empty strategy", &ProjectConfig{Env: EnvConfig{Strategy: ""}}},
		{"valid setup steps", &ProjectConfig{
			Setup: []SetupStep{{Name: "test", Run: "echo hi"}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, ValidateProjectConfig(tt.cfg))
		})
	}
}

func TestValidateProjectConfig_InvalidStrategy(t *testing.T) {
	cfg := &ProjectConfig{Env: EnvConfig{Strategy: "move"}}
	err := ValidateProjectConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid env strategy")
}

func TestValidateProjectConfig_MissingRunCommand(t *testing.T) {
	cfg := &ProjectConfig{
		Setup: []SetupStep{{Name: "bad step", Run: ""}},
	}
	err := ValidateProjectConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "run command is required")
}

func TestGenerateDefaultConfig_Defaults(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{})
	assert.Contains(t, out, `default_base = "main"`)
	assert.Contains(t, out, `strategy = "symlink"`)
	assert.Contains(t, out, `".env", ".env.local"`)
	assert.NotContains(t, out, "[[setup]]")
	assert.Contains(t, out, "[hooks]")
}

func TestGenerateDefaultConfig_WithBranch(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{DefaultBase: "develop"})
	assert.Contains(t, out, `default_base = "develop"`)
}

func TestGenerateDefaultConfig_WithEnvFiles(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{
		EnvFiles: []string{".env", ".env.staging"},
	})
	assert.Contains(t, out, `strategy = "symlink"`)
	assert.Contains(t, out, `".env", ".env.staging"`)
}

func TestGenerateDefaultConfig_DefaultEnvFilesWhenNoneDetected(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{})
	assert.Contains(t, out, `strategy = "symlink"`)
	assert.Contains(t, out, `".env", ".env.local"`)
}

func TestGenerateDefaultConfig_WithInstallCmd(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{
		InstallCmd: "pnpm install",
	})
	assert.Contains(t, out, "[[setup]]")
	assert.Contains(t, out, `name = "Install dependencies"`)
	assert.Contains(t, out, `run = "pnpm install"`)
}

func TestGenerateDefaultConfig_NoSetupWithoutInstallCmd(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{})
	assert.NotContains(t, out, "[[setup]]")
}

func TestGenerateDefaultConfig_IsValidTOML(t *testing.T) {
	out := GenerateDefaultConfig(DefaultConfigOptions{
		InstallCmd: "npm install",
	})

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ProjectConfigFile), []byte(out), 0o644))

	cfg, err := LoadProjectConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "main", cfg.Worktree.DefaultBase)
	assert.Equal(t, "symlink", cfg.Env.Strategy)
	assert.Equal(t, []string{".env", ".env.local"}, cfg.Env.Files)
	require.Len(t, cfg.Setup, 1)
	assert.Equal(t, "Install dependencies", cfg.Setup[0].Name)
	assert.Equal(t, "npm install", cfg.Setup[0].Run)
}
