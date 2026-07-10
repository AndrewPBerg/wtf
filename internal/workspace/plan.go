package workspace

import (
	"fmt"
	"path/filepath"
)

// Plan is a side-effect-free rendering of an environment instance. Execution,
// resource leasing, and process management intentionally remain separate from
// configuration parsing during this spike.
type Plan struct {
	Workspace string        `json:"workspace"`
	Profile   string        `json:"profile"`
	Instance  string        `json:"instance"`
	Services  []PlanService `json:"services"`
}

// PlanService is one configured service resolved against a workspace root.
type PlanService struct {
	Name        string            `json:"name"`
	Dir         string            `json:"dir"`
	Source      string            `json:"source"`
	WorktreeFor string            `json:"worktree_for,omitempty"`
	Env         *Environment      `json:"env,omitempty"`
	Ports       map[string]Port   `json:"ports,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Up          string            `json:"up,omitempty"`
}

// BuildPlan resolves one named profile for an environment instance. An
// instance is normally a branch name, but WTF does not impose that meaning on
// a profile: it is only a stable namespace for subsequent execution.
func BuildPlan(config *Config, workspaceDir, profileName, instance string) (*Plan, error) {
	if instance == "" {
		return nil, fmt.Errorf("instance cannot be empty")
	}
	profile, ok := config.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	plan := &Plan{
		Workspace: workspaceDir,
		Profile:   profileName,
		Instance:  instance,
		Services:  make([]PlanService, 0, len(profile.Services)),
	}
	for _, name := range profile.Services {
		service := config.Services[name]
		source := service.Source
		if source == "" {
			source = "worktree"
		}
		planned := PlanService{
			Name:        name,
			Dir:         filepath.Clean(filepath.Join(workspaceDir, service.Dir)),
			Source:      source,
			Env:         service.Env,
			Ports:       service.Ports,
			Environment: service.EnvVars,
			Up:          service.Up,
		}
		if source == "worktree" {
			planned.WorktreeFor = instance
		}
		plan.Services = append(plan.Services, planned)
	}
	return plan, nil
}
