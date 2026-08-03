package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/vcs"
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

// registryVersion is the current on-disk schema version.
const registryVersion = 2

// repoEntry is one registered repo plus the backend the user chose for it.
type repoEntry struct {
	Path string `json:"path"`
	// VCS records which backend to use in a colocated repo, where both .git and
	// .jj are present and wtf would otherwise have to ask every time. Empty
	// means "not decided" — detection applies.
	VCS vcs.Kind `json:"vcs,omitempty"`
}

// registry is the v2 on-disk format.
type registry struct {
	Version int         `json:"version"`
	Repos   []repoEntry `json:"repos"`
}

// readRegistry loads the registry, transparently upgrading the legacy format.
//
// v1 was a bare JSON array of paths. It is still read so existing installs keep
// working; the file is only rewritten in the new shape on the next Save.
func readRegistry() (registry, error) {
	data, err := os.ReadFile(RegistryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return registry{Version: registryVersion}, nil
		}
		return registry{}, fmt.Errorf("reading registry: %w", err)
	}

	var reg registry
	if err := json.Unmarshal(data, &reg); err == nil && reg.Repos != nil {
		return reg, nil
	}

	// Legacy v1: ["/path/a", "/path/b"]
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return registry{}, fmt.Errorf("parsing registry: %w", err)
	}
	reg = registry{Version: registryVersion}
	for _, p := range paths {
		reg.Repos = append(reg.Repos, repoEntry{Path: p})
	}
	return reg, nil
}

// writeRegistry persists the registry in the current format.
func writeRegistry(reg registry) error {
	dir := DefaultWTFHome()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating wtf home: %w", err)
	}

	reg.Version = registryVersion
	if reg.Repos == nil {
		reg.Repos = []repoEntry{}
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}

	if err := os.WriteFile(RegistryPath(), data, 0o644); err != nil {
		return fmt.Errorf("writing registry: %w", err)
	}
	return nil
}

// Load reads the registry and returns all stored repo paths.
// Returns an empty slice if the file does not exist.
func Load() ([]string, error) {
	reg, err := readRegistry()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(reg.Repos))
	for _, e := range reg.Repos {
		paths = append(paths, e.Path)
	}
	return paths, nil
}

// Save writes the given repo paths to the registry file, preserving any recorded
// backend preferences for paths that survive.
func Save(paths []string) error {
	reg, err := readRegistry()
	if err != nil {
		return err
	}

	prefs := make(map[string]vcs.Kind, len(reg.Repos))
	for _, e := range reg.Repos {
		if e.VCS != "" {
			prefs[e.Path] = e.VCS
		}
	}

	next := registry{Version: registryVersion}
	for _, p := range paths {
		next.Repos = append(next.Repos, repoEntry{Path: p, VCS: prefs[p]})
	}
	return writeRegistry(next)
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

// Remove unregisters a repo path. Returns true if it was found and removed.
func Remove(repoPath string) (bool, error) {
	paths, err := Load()
	if err != nil {
		return false, err
	}

	var filtered []string
	found := false
	for _, p := range paths {
		if p == repoPath {
			found = true
			continue
		}
		filtered = append(filtered, p)
	}

	if !found {
		return false, nil
	}

	return true, Save(filtered)
}

// VCSPref returns the backend recorded for a repo, if the user has chosen one.
// Only colocated repos ever need this; elsewhere detection is unambiguous.
func VCSPref(repoPath string) (vcs.Kind, bool) {
	reg, err := readRegistry()
	if err != nil {
		return "", false
	}
	for _, e := range reg.Repos {
		if e.Path == repoPath && e.VCS != "" {
			return e.VCS, true
		}
	}
	return "", false
}

// SetVCSPref records which backend to use for a repo, registering the repo if it
// is not already known.
func SetVCSPref(repoPath string, kind vcs.Kind) error {
	reg, err := readRegistry()
	if err != nil {
		return err
	}

	for i := range reg.Repos {
		if reg.Repos[i].Path == repoPath {
			reg.Repos[i].VCS = kind
			return writeRegistry(reg)
		}
	}

	reg.Repos = append(reg.Repos, repoEntry{Path: repoPath, VCS: kind})
	return writeRegistry(reg)
}

// ClearVCSPref forgets the recorded backend for a repo, so the next command in a
// colocated repo asks again.
func ClearVCSPref(repoPath string) error {
	reg, err := readRegistry()
	if err != nil {
		return err
	}
	changed := false
	for i := range reg.Repos {
		if reg.Repos[i].Path == repoPath && reg.Repos[i].VCS != "" {
			reg.Repos[i].VCS = ""
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeRegistry(reg)
}

// LoadValid returns only the registered paths that still exist and are repos.
// Unlike Prune, it does not modify the registry file.
func LoadValid() ([]string, error) {
	paths, err := Load()
	if err != nil {
		return nil, err
	}

	var valid []string
	for _, p := range paths {
		if IsRepo(p) {
			valid = append(valid, p)
		}
	}
	return valid, nil
}

// Prune removes stale entries (paths that no longer exist or aren't repos)
// and returns the valid paths. This writes the cleaned list back to the registry.
func Prune() ([]string, error) {
	valid, err := LoadValid()
	if err != nil {
		return nil, err
	}

	if err := Save(valid); err != nil {
		return nil, err
	}
	return valid, nil
}

// IsRepo reports whether path is the *primary* checkout of a repo wtf can
// manage. jj workspaces carry no .git at all, so checking only for .git would
// make every jj repo look invalid.
//
// Secondary checkouts are deliberately rejected so they never accumulate in the
// registry: a git worktree has .git as a file rather than a directory, and a
// secondary jj workspace likewise has .jj/repo as a file pointer rather than the
// repo directory itself.
func IsRepo(path string) bool {
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(path, ".jj", "repo")); err == nil && info.IsDir() {
		return true
	}
	return false
}
