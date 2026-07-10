package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPlan(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".wtf"), 0o755))
	config := `version: 1
profiles:
  fullstack:
    services: [api]
services:
  api:
    dir: api
    env: {path: .env, mode: copy}
    ports:
      http: {from: 8000, to: 8099}
    up: uv run python manage.py runserver 0.0.0.0:$PORT
`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".wtf", "workspace.yaml"), []byte(config), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "api"), 0o755))
	t.Chdir(filepath.Join(root, "api"))

	out := new(bytes.Buffer)
	cmd := planCmd
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	require.NoError(t, runPlan(cmd, "fullstack", "feature/auth"))
	assert.Contains(t, out.String(), "instance feature/auth")
	assert.Contains(t, out.String(), "api")
	assert.Contains(t, out.String(), "8000-8099")
	assert.Contains(t, out.String(), "copy")
}

func TestRunPlanJSON(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".wtf"), 0o755))
	config := "version: 1\nprofiles: {dev: {services: [app]}}\nservices: {app: {dir: app}}\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".wtf", "workspace.yaml"), []byte(config), 0o600))
	t.Chdir(root)

	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })
	out := new(bytes.Buffer)
	cmd := planCmd
	cmd.SetOut(out)

	require.NoError(t, runPlan(cmd, "dev", "one"))
	assert.Contains(t, out.String(), `"profile": "dev"`)
	assert.Contains(t, out.String(), `"instance": "one"`)
}
