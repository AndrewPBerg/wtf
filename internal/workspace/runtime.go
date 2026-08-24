package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	gitpkg "github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
)

// RuntimeState is the persisted lifecycle state for one environment instance.
type RuntimeState struct {
	Profile  string           `json:"profile"`
	Instance string           `json:"instance"`
	Services []RunningService `json:"services"`
}

// RunningService records one materialized service and its assigned resources.
type RunningService struct {
	Name     string         `json:"name"`
	Dir      string         `json:"dir"`
	Worktree string         `json:"worktree,omitempty"`
	Ports    map[string]int `json:"ports,omitempty"`
	PID      int            `json:"pid,omitempty"`
	Log      string         `json:"log,omitempty"`
}

// Up materializes the worktree and environment-file portion of a plan, leases
// configured ports, and starts each declared command. State is persisted below
// .wtf/state so Down can stop the same environment instance later.
func Up(config *Config, root, profile, instance string) (*RuntimeState, error) {
	var state *RuntimeState
	err := withLock(root, func() error {
		var err error
		state, err = up(config, root, profile, instance)
		return err
	})
	return state, err
}

func up(config *Config, root, profile, instance string) (*RuntimeState, error) {
	plan, err := BuildPlan(config, root, profile, instance)
	if err != nil {
		return nil, err
	}
	if _, err := loadState(root, instance); err == nil {
		return nil, fmt.Errorf("instance %q is already up", instance)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	resources, err := loadResources(root)
	if err != nil {
		return nil, err
	}
	state := &RuntimeState{Profile: profile, Instance: instance}
	completed := false
	defer func() {
		if !completed {
			for _, service := range state.Services {
				if service.PID > 0 {
					_ = stopProcess(service.PID)
				}
			}
		}
	}()
	wm := gitpkg.NewWorktreeManager(&gitpkg.RealExecutor{})
	envHandler := setup.NewEnvFileHandler()
	for _, service := range plan.Services {
		target := service.Dir
		running := RunningService{Name: service.Name, Ports: map[string]int{}}
		if service.Source == "worktree" {
			wt, findErr := wm.Find(service.Dir, instance)
			if findErr == nil {
				target = wt.Path
			} else {
				main, mainErr := wm.MainWorktree(service.Dir)
				if mainErr != nil {
					return nil, fmt.Errorf("finding base branch for %s: %w", service.Name, mainErr)
				}
				if main.Branch == "" {
					return nil, fmt.Errorf("finding base branch for %s: main worktree is detached", service.Name)
				}
				created, addErr := wm.Add(service.Dir, instance, main.Branch)
				if addErr != nil {
					return nil, fmt.Errorf("creating worktree for %s: %w", service.Name, addErr)
				}
				target = created
			}
			running.Worktree = target
		}
		setupRunner := setup.NewRunner()
		setupRunner.CmdExec = setupExecutor{}
		if err := setupRunner.RunSetup(service.Dir, target, setup.Options{SkipEnv: true}); err != nil {
			return nil, fmt.Errorf("installing dependencies for %s: %w", service.Name, err)
		}
		if service.Env != nil {
			if _, err := envHandler.HandleEnvFiles(service.Dir, target, service.Env.Mode, []string{service.Env.Path}); err != nil {
				return nil, fmt.Errorf("materializing env for %s: %w", service.Name, err)
			}
		}
		for name, r := range service.Ports {
			key := instance + ":" + service.Name + ":" + name
			p, err := leasePort(resources, key, r)
			if err != nil {
				return nil, err
			}
			running.Ports[name] = p
		}
		masked := map[string]string{}
		if service.Env != nil {
			for key, value := range service.Env.Set {
				masked[key] = expand(value, instance, resources)
			}
			if err := upsertDotenv(filepath.Join(target, service.Env.Path), masked); err != nil {
				return nil, fmt.Errorf("masking env for %s: %w", service.Name, err)
			}
		}
		if service.Up != "" {
			pid, log, err := start(target, service, running.Ports, masked, root, instance)
			if err != nil {
				return nil, fmt.Errorf("starting %s: %w", service.Name, err)
			}
			running.PID, running.Log = pid, log
		}
		running.Dir = target
		state.Services = append(state.Services, running)
	}
	if err := saveResources(root, resources); err != nil {
		return nil, err
	}
	if err := saveState(root, state); err != nil {
		return nil, err
	}
	if err := Register(root); err != nil {
		return nil, fmt.Errorf("registering workspace: %w", err)
	}
	completed = true
	return state, nil
}

// Down stops recorded processes and releases the resources for an instance.
func Down(root, instance string) error {
	return withLock(root, func() error { return down(root, instance) })
}

func down(root, instance string) error {
	state, err := loadState(root, instance)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("instance %q is not up", instance)
		}
		return err
	}
	resources, err := loadResources(root)
	if err != nil {
		return err
	}
	for _, service := range state.Services {
		if service.PID > 0 {
			_ = stopProcess(service.PID)
		}
		for name := range service.Ports {
			delete(resources, instance+":"+service.Name+":"+name)
		}
	}
	if err := saveResources(root, resources); err != nil {
		return err
	}
	return os.Remove(statePath(root, instance))
}

func leasePort(resources map[string]int, key string, r Port) (int, error) {
	if p, ok := resources[key]; ok {
		return p, nil
	}
	used := map[int]bool{}
	for _, p := range resources {
		used[p] = true
	}
	for p := r.From; p <= r.To; p++ {
		if !used[p] && portAvailable(p) {
			resources[key] = p
			return p, nil
		}
	}
	return 0, fmt.Errorf("no ports available for %s", key)
}
func portAvailable(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

func expand(value, instance string, resources map[string]int) string {
	value = strings.ReplaceAll(value, "${instance}", strings.ReplaceAll(instance, "/", "_"))
	for key, port := range resources {
		parts := strings.Split(key, ":")
		if len(parts) == 3 && parts[0] == instance {
			value = strings.ReplaceAll(value, "${port."+parts[2]+"}", strconv.Itoa(port))
		}
	}
	return value
}

func upsertDotenv(path string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		for key, value := range values {
			if strings.HasPrefix(line, key+"=") {
				lines[i] = key + "=" + value
				seen[key] = true
			}
		}
	}
	for key, value := range values {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

func start(dir string, service PlanService, ports map[string]int, masked map[string]string, root, instance string) (int, string, error) {
	log := filepath.Join(root, ".wtf", "state", "logs", stateID(instance), service.Name+".log")
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		return 0, "", err
	}
	f, err := os.OpenFile(log, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, "", err
	}
	cmd := exec.Command("sh", "-c", service.Up)
	cmd.Dir = dir
	isolateProcess(cmd)
	cmd.Stdout, cmd.Stderr = f, f
	cmd.Env = os.Environ()
	for key, value := range masked {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	names := make([]string, 0, len(ports))
	for n := range ports {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		value := strconv.Itoa(ports[n])
		cmd.Env = append(cmd.Env, "WTF_PORT_"+strings.ToUpper(strings.ReplaceAll(n, "-", "_"))+"="+value)
		if n == "http" || len(names) == 1 {
			cmd.Env = append(cmd.Env, "PORT="+value)
		}
	}
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return 0, "", err
	}
	_ = f.Close()
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, log, nil
}
func stateID(instance string) string {
	sum := sha256.Sum256([]byte(instance))
	return hex.EncodeToString(sum[:8])
}
func statePath(root, instance string) string {
	return filepath.Join(root, ".wtf", "state", "instances", stateID(instance)+".json")
}
func resourcePath(root string) string { return filepath.Join(root, ".wtf", "state", "resources.json") }
func loadState(root, instance string) (*RuntimeState, error) {
	b, err := os.ReadFile(statePath(root, instance))
	if err != nil {
		return nil, err
	}
	var s RuntimeState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
func saveState(root string, s *RuntimeState) error { return saveJSON(statePath(root, s.Instance), s) }
func loadResources(root string) (map[string]int, error) {
	b, err := os.ReadFile(resourcePath(root))
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var r map[string]int
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return r, nil
}
func saveResources(root string, r map[string]int) error { return saveJSON(resourcePath(root), r) }
func saveJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
