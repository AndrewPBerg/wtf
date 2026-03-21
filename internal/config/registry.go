package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultWTFHome returns the default WTF home directory (~/.wtf).
// Override with WTF_HOME env var for testing.
func DefaultWTFHome() string {
	if home := os.Getenv("WTF_HOME"); home != "" {
		return home
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".wtf")
	}
	return filepath.Join(homeDir, ".wtf")
}

// RegistryPath returns the path to the repos registry file.
func RegistryPath() string {
	return filepath.Join(DefaultWTFHome(), "repos.json")
}

// Load reads the registry and returns all stored repo paths.
// Returns an empty slice if the file does not exist.
func Load() ([]string, error) {
	data, err := os.ReadFile(RegistryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading registry: %w", err)
	}

	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	return paths, nil
}

// Save writes the given repo paths to the registry file.
func Save(paths []string) error {
	dir := DefaultWTFHome()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating wtf home: %w", err)
	}

	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}

	if err := os.WriteFile(RegistryPath(), data, 0o644); err != nil {
		return fmt.Errorf("writing registry: %w", err)
	}
	return nil
}

// Add registers a repo path if not already present.
func Add(repoPath string) error {
	paths, err := Load()
	if err != nil {
		return err
	}

	for _, p := range paths {
		if p == repoPath {
			return nil // already registered
		}
	}

	paths = append(paths, repoPath)
	return Save(paths)
}

// Prune removes stale entries (paths that no longer exist or aren't git repos)
// and returns the valid paths.
func Prune() ([]string, error) {
	paths, err := Load()
	if err != nil {
		return nil, err
	}

	var valid []string
	for _, p := range paths {
		if isGitRepo(p) {
			valid = append(valid, p)
		}
	}

	if err := Save(valid); err != nil {
		return nil, err
	}
	return valid, nil
}

// isGitRepo checks if the path exists and contains a .git directory.
func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}
