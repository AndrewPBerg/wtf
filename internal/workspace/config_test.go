package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfig = `version: 1
profiles:
  fullstack:
    services: [web, api]
services:
  web:
    dir: web
    ports:
      http: {from: 3000, to: 3099}
    up: pnpm dev -- --port $PORT
  api:
    dir: api
    env: {path: .env, mode: copy}
    ports:
      http: {from: 8000, to: 8099}
    up: uv run python manage.py runserver 0.0.0.0:$PORT
`

func TestLoadFindsParentWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".wtf")
	require.NoError(t, os.Mkdir(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "workspace.yaml"), []byte(validConfig), 0o600))

	child := filepath.Join(root, "api", "nested")
	require.NoError(t, os.MkdirAll(child, 0o755))
	config, gotRoot, err := Load(child)

	require.NoError(t, err)
	assert.Equal(t, root, gotRoot)
	assert.Len(t, config.Profiles, 1)
	assert.Equal(t, "api", config.Services["api"].Dir)
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".wtf")
	require.NoError(t, os.Mkdir(configDir, 0o755))
	contents := "version: 1\nprofiles: {app: {services: [app]}}\nservices: {app: {dir: app, typo: no}}\n"
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "workspace.yaml"), []byte(contents), 0o600))

	_, _, err := Load(root)

	assert.ErrorContains(t, err, "field typo not found")
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".wtf")
	require.NoError(t, os.Mkdir(configDir, 0o755))
	contents := validConfig + "---\nversion: 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "workspace.yaml"), []byte(contents), 0o600))

	_, _, err := Load(root)

	assert.ErrorContains(t, err, "single YAML document")
}

func TestLoadRejectsConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".wtf", "workspace.yaml")
	require.NoError(t, os.MkdirAll(configPath, 0o755))

	_, _, err := Load(root)

	assert.ErrorContains(t, err, "is a directory")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name:    "unknown profile service",
			config:  Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"missing"}}}, Services: map[string]Service{"other": {Dir: "other"}}},
			wantErr: `references unknown service "missing"`,
		},
		{
			name:    "absolute service directory",
			config:  Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"app"}}}, Services: map[string]Service{"app": {Dir: "/app"}}},
			wantErr: "must be relative",
		},
		{
			name:    "service directory cannot escape workspace",
			config:  Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"app"}}}, Services: map[string]Service{"app": {Dir: "../app"}}},
			wantErr: "must be relative",
		},
		{
			name:   "workspace root is a valid service directory",
			config: Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"app"}}}, Services: map[string]Service{"app": {Dir: "."}}},
		},
		{
			name:    "directory sources cannot materialize env files",
			config:  Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"app"}}}, Services: map[string]Service{"app": {Dir: "app", Source: "directory", Env: &Environment{Path: ".env", Mode: "symlink"}}}},
			wantErr: "cannot materialize env files",
		},
		{
			name:   "worktree symlink is supported",
			config: Config{Version: 1, Profiles: map[string]Profile{"dev": {Services: []string{"app"}}}, Services: map[string]Service{"app": {Dir: "app", Source: "worktree", Env: &Environment{Path: ".env", Mode: "symlink"}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
