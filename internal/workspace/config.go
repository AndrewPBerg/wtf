// Package workspace defines profile-driven local development environments.
package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configDirName = ".wtf"
const configFileName = "workspace.yaml"

// Config is the committed description of a local development workspace.
type Config struct {
	Version  int                `yaml:"version" json:"version"`
	Profiles map[string]Profile `yaml:"profiles" json:"profiles"`
	Services map[string]Service `yaml:"services" json:"services"`
}

// Profile selects the services that form an environment.
type Profile struct {
	Services []string `yaml:"services" json:"services"`
}

// Service describes one repository or directory participating in a profile.
// WTF deliberately treats Up as an opaque command; framework knowledge remains
// in the workspace configuration.
type Service struct {
	Dir     string            `yaml:"dir" json:"dir"`
	Source  string            `yaml:"source" json:"source"` // worktree (default) or directory
	Env     *Environment      `yaml:"env" json:"env"`
	Ports   map[string]Port   `yaml:"ports" json:"ports"`
	Up      string            `yaml:"up" json:"up"`
	EnvVars map[string]string `yaml:"environment" json:"environment"`
}

// Environment materializes and optionally masks one dotenv file.
type Environment struct {
	Path string            `yaml:"path" json:"path"`
	Mode string            `yaml:"mode" json:"mode"` // copy or symlink
	Set  map[string]string `yaml:"set" json:"set"`
}

// Port declares a named service port and its allocation range.
type Port struct {
	From int `yaml:"from" json:"from"`
	To   int `yaml:"to" json:"to"`
}

// Load searches dir and its parents for .wtf/workspace.yaml, then decodes and
// validates it. The returned directory is the workspace root (the parent of
// .wtf), not the directory in which the command was invoked.
func Load(dir string) (*Config, string, error) {
	root, path, err := findConfig(dir)
	if err != nil {
		return nil, "", err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("opening workspace config: %w", err)
	}
	defer func() { _ = file.Close() }()

	config, err := decode(file)
	if err != nil {
		return nil, "", err
	}
	if err := config.Validate(); err != nil {
		return nil, "", err
	}
	return config, root, nil
}

func findConfig(dir string) (string, string, error) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolving workspace directory: %w", err)
	}

	for {
		path := filepath.Join(current, configDirName, configFileName)
		info, statErr := os.Stat(path)
		switch {
		case statErr == nil && info.IsDir():
			return "", "", fmt.Errorf("workspace config %s is a directory", path)
		case statErr == nil:
			return current, path, nil
		case !os.IsNotExist(statErr):
			return "", "", fmt.Errorf("checking workspace config %s: %w", path, statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("no %s/%s found from %s", configDirName, configFileName, dir)
		}
		current = parent
	}
}

func decode(r io.Reader) (*Config, error) {
	var config Config
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parsing workspace config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("workspace config must contain a single YAML document")
		}
		return nil, fmt.Errorf("parsing workspace config: %w", err)
	}
	return &config, nil
}

// Validate verifies the schema and cross-references without inspecting or
// modifying service directories.
func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("workspace config version must be 1")
	}
	if len(c.Profiles) == 0 {
		return fmt.Errorf("workspace config must declare at least one profile")
	}
	if len(c.Services) == 0 {
		return fmt.Errorf("workspace config must declare at least one service")
	}

	for name, profile := range c.Profiles {
		if name == "" {
			return fmt.Errorf("profile name cannot be empty")
		}
		if len(profile.Services) == 0 {
			return fmt.Errorf("profile %q must declare at least one service", name)
		}
		seen := make(map[string]bool, len(profile.Services))
		for _, service := range profile.Services {
			if seen[service] {
				return fmt.Errorf("profile %q declares service %q more than once", name, service)
			}
			seen[service] = true
			if _, ok := c.Services[service]; !ok {
				return fmt.Errorf("profile %q references unknown service %q", name, service)
			}
		}
	}

	for name, service := range c.Services {
		if name == "" {
			return fmt.Errorf("service name cannot be empty")
		}
		if service.Dir == "" {
			return fmt.Errorf("service %q must declare dir", name)
		}
		if !isWorkspaceDir(service.Dir) {
			return fmt.Errorf("service %q dir must be relative to the workspace root", name)
		}
		if service.Source != "" && service.Source != "worktree" && service.Source != "directory" {
			return fmt.Errorf("service %q has unsupported source %q", name, service.Source)
		}
		if service.Source == "directory" && service.Env != nil {
			return fmt.Errorf("service %q cannot materialize env files for directory source", name)
		}
		if env := service.Env; env != nil {
			if !isWorkspaceRelative(env.Path) {
				return fmt.Errorf("service %q env path must be relative", name)
			}
			if env.Mode != "copy" && env.Mode != "symlink" {
				return fmt.Errorf("service %q env has unsupported mode %q", name, env.Mode)
			}
			if env.Mode == "symlink" && len(env.Set) > 0 {
				return fmt.Errorf("service %q cannot set values in a symlinked env file", name)
			}
		}
		for portName, port := range service.Ports {
			if portName == "" || port.From < 1 || port.To < port.From || port.To > 65535 {
				return fmt.Errorf("service %q port %q has invalid range", name, portName)
			}
		}
	}
	return nil
}

func isWorkspaceDir(path string) bool {
	return isSafeRelative(path, true)
}

func isWorkspaceRelative(path string) bool {
	return isSafeRelative(path, false)
}

func isSafeRelative(path string, allowCurrentDir bool) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	if allowCurrentDir && clean == "." {
		return true
	}
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
