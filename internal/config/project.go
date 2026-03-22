package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// ProjectConfigFile is the config file name.
const ProjectConfigFile = ".wt-forge.toml"

// ProjectConfig represents the project-level configuration.
type ProjectConfig struct {
	Worktree WorktreeConfig `toml:"worktree"`
	Env      EnvConfig      `toml:"env"`
	Setup    []SetupStep    `toml:"setup"`
	Hooks    HooksConfig    `toml:"hooks"`
}

// WorktreeConfig configures worktree behavior.
type WorktreeConfig struct {
	RootPath    string `toml:"root_path"`
	DefaultBase string `toml:"default_base"`
}

// EnvConfig configures env file handling.
type EnvConfig struct {
	Strategy string   `toml:"strategy"` // "symlink" | "copy" | "none"
	Files    []string `toml:"files"`
}

// SetupStep represents a single setup step.
type SetupStep struct {
	Name string `toml:"name"`
	Run  string `toml:"run"`
	If   string `toml:"if"`
}

// HooksConfig configures lifecycle hooks.
type HooksConfig struct {
	OnCreate []string `toml:"on_create"`
	OnSwitch []string `toml:"on_switch"`
	OnRemove []string `toml:"on_remove"`
}

// LoadProjectConfig loads the project config from the given directory.
// Returns nil, nil if the config file does not exist.
func LoadProjectConfig(dir string) (*ProjectConfig, error) {
	path := filepath.Join(dir, ProjectConfigFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading project config: %w", err)
	}

	var cfg ProjectConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing project config: %w", err)
	}

	return &cfg, nil
}

// DefaultEnvFiles is the default list of env files when none are detected.
var DefaultEnvFiles = []string{".env", ".env.local"}

// DefaultConfigOptions holds parameters for generating a default config.
type DefaultConfigOptions struct {
	DefaultBase string   // e.g. "main", "master"
	EnvFiles    []string // env files found in the repo; falls back to DefaultEnvFiles
	InstallCmd  string   // e.g. "npm install", "" if none detected
}

// GenerateDefaultConfig returns a .wt-forge.toml with sensible defaults.
func GenerateDefaultConfig(opts DefaultConfigOptions) string {
	if opts.DefaultBase == "" {
		opts.DefaultBase = "main"
	}

	envFiles := opts.EnvFiles
	if len(envFiles) == 0 {
		envFiles = DefaultEnvFiles
	}

	var b strings.Builder

	b.WriteString("[worktree]\n")
	b.WriteString(fmt.Sprintf("default_base = %q\n", opts.DefaultBase))

	b.WriteString("\n[env]\n")
	b.WriteString("strategy = \"symlink\"\n")
	b.WriteString("files = [")
	for i, f := range envFiles {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%q", f))
	}
	b.WriteString("]\n")

	if opts.InstallCmd != "" {
		b.WriteString("\n[[setup]]\n")
		b.WriteString("name = \"Install dependencies\"\n")
		b.WriteString(fmt.Sprintf("run = %q\n", opts.InstallCmd))
	}

	b.WriteString("\n[hooks]\n")
	b.WriteString("# on_create = []\n")
	b.WriteString("# on_switch = []\n")
	b.WriteString("# on_remove = []\n")

	return b.String()
}

// ValidateProjectConfig validates the project configuration.
func ValidateProjectConfig(cfg *ProjectConfig) error {
	if cfg == nil {
		return nil
	}

	switch cfg.Env.Strategy {
	case "", "symlink", "copy", "none":
		// valid
	default:
		return fmt.Errorf("invalid env strategy: %q (must be symlink, copy, or none)", cfg.Env.Strategy)
	}

	for i, step := range cfg.Setup {
		if step.Run == "" {
			return fmt.Errorf("setup step %d (%q): run command is required", i, step.Name)
		}
	}

	return nil
}
