package workspace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlan(t *testing.T) {
	config := &Config{
		Version: 1,
		Profiles: map[string]Profile{
			"fullstack": {Services: []string{"web", "api"}},
		},
		Services: map[string]Service{
			"web": {Dir: "web", Ports: map[string]Port{"http": {From: 3000, To: 3099}}},
			"api": {Dir: "api", Source: "directory", Up: "uv run python manage.py runserver 0.0.0.0:$PORT"},
		},
	}

	plan, err := BuildPlan(config, "/code/acme", "fullstack", "feature/auth")

	require.NoError(t, err)
	assert.Equal(t, "/code/acme", plan.Workspace)
	assert.Equal(t, "feature/auth", plan.Instance)
	require.Len(t, plan.Services, 2)
	assert.Equal(t, filepath.Join("/code/acme", "web"), plan.Services[0].Dir)
	assert.Equal(t, "feature/auth", plan.Services[0].WorktreeFor)
	assert.Equal(t, 3000, plan.Services[0].Ports["http"].From)
	assert.Equal(t, "api", plan.Services[1].Name)
	assert.Equal(t, "directory", plan.Services[1].Source)
	assert.Empty(t, plan.Services[1].WorktreeFor)
}

func TestBuildPlanErrors(t *testing.T) {
	config := &Config{Profiles: map[string]Profile{"dev": {Services: []string{}}}}

	_, err := BuildPlan(config, "/workspace", "missing", "feature/auth")
	assert.ErrorContains(t, err, `profile "missing" not found`)

	_, err = BuildPlan(config, "/workspace", "dev", "")
	assert.ErrorContains(t, err, "instance cannot be empty")
}
