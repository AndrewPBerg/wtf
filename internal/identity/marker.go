package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const repositoryMarkerName = "repository-id"

// RepositoryMarkerPath returns the marker location owned by a VCS backend.
func RepositoryMarkerPath(stateDir string) string {
	return filepath.Join(stateDir, repositoryMarkerName)
}

// validateStateDir requires the backend-owned marker directory to be an
// absolute, local physical path. Unlike workspace paths, silently resolving a
// symlink here could make two backends believe they own different markers.
func validateStateDir(stateDir string) (string, error) {
	if stateDir == "" || !filepath.IsAbs(stateDir) || strings.ContainsRune(stateDir, '\x00') || strings.Contains(stateDir, "://") {
		return "", fmt.Errorf("state directory must be an absolute local path")
	}
	canonical, err := canonicalPath(stateDir, "state directory")
	if err != nil {
		return "", err
	}
	if canonical != filepath.Clean(stateDir) {
		return "", fmt.Errorf("state directory must not contain symlinks")
	}
	return canonical, nil
}

func markerLockPath(stateDir string) string { return RepositoryMarkerPath(stateDir) + ".lock" }

// ReadRepositoryID reads and validates a backend repository marker. Marker
// contents are deliberately only the canonical UUID (with an optional final newline).
func ReadRepositoryID(stateDir string) (string, error) {
	stateDir, err := validateStateDir(stateDir)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(RepositoryMarkerPath(stateDir))
	if err != nil {
		return "", fmt.Errorf("reading repository marker: %w", err)
	}
	id := strings.TrimSuffix(string(b), "\n")
	if strings.ContainsAny(id, "\r\n") {
		return "", fmt.Errorf("repository marker is malformed")
	}
	if err := ValidateID(id); err != nil {
		return "", fmt.Errorf("invalid repository marker: %w", err)
	}
	return id, nil
}

// WriteRepositoryID atomically creates or verifies a backend marker. It never
// overwrites a different identity.
func WriteRepositoryID(stateDir, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	stateDir, err := validateStateDir(stateDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("creating repository marker directory: %w", err)
	}
	unlock, err := lockFile(markerLockPath(stateDir))
	if err != nil {
		return fmt.Errorf("locking repository marker: %w", err)
	}
	defer func() { _ = unlock() }()
	path := RepositoryMarkerPath(stateDir)
	if existing, err := ReadRepositoryID(stateDir); err == nil {
		if existing != id {
			return fmt.Errorf("repository marker belongs to %q", existing)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return persistMarker(path, []byte(id+"\n"))
}

// EnsureRepository reconciles the backend marker and global identity. The
// marker is authoritative only when it is valid and its locator agrees with
// global state; missing global state is repaired using the marker.
func (s *Store) EnsureRepository(locator, stateDir string) (Repository, error) {
	stateDir, err := validateStateDir(stateDir)
	if err != nil {
		return Repository{}, err
	}
	canonical, err := canonicalPath(locator, "repository locator")
	if err != nil {
		return Repository{}, err
	}
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return Repository{}, err
	}
	unlock, err := lockFile(s.lockPath)
	if err != nil {
		return Repository{}, err
	}
	defer func() { _ = unlock() }()
	state, err := readState(s.statePath)
	if err != nil {
		return Repository{}, err
	}
	marker, markerErr := ReadRepositoryID(stateDir)
	if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		return Repository{}, markerErr
	}
	var result Repository
	for _, r := range state.Repositories {
		if r.LifecycleState == Active && r.Locator == canonical {
			if markerErr == nil && marker != r.ID {
				return Repository{}, fmt.Errorf("repository marker conflicts with global state")
			}
			// Publish a missing marker before returning. This is deliberately
			// done before any future state publication so a failed marker write
			// can never leave global identity ahead of its durable marker.
			if markerErr != nil {
				if err := writeMarkerLocked(stateDir, r.ID); err != nil {
					return Repository{}, err
				}
			}
			return r, nil
		}
	}
	if markerErr == nil {
		for _, r := range state.Repositories {
			if r.ID == marker {
				if r.Locator != canonical {
					return Repository{}, fmt.Errorf("repository marker locator mismatch")
				}
				return r, nil
			}
		}
		id := marker
		t := now()
		result = Repository{ID: id, Locator: canonical, LifecycleState: Active, CreatedAt: t, UpdatedAt: t}
	} else {
		id, e := NewID()
		if e != nil {
			return Repository{}, e
		}
		t := now()
		result = Repository{ID: id, Locator: canonical, LifecycleState: Active, CreatedAt: t, UpdatedAt: t}
		// The marker is the recovery anchor. Publish it first; if this fails,
		// leave the missing global state untouched.
		if err := writeMarkerLocked(stateDir, result.ID); err != nil {
			return Repository{}, err
		}
	}
	state.Repositories = append(state.Repositories, result)
	if err := state.Validate(); err != nil {
		return Repository{}, err
	}
	if err := writeState(s.statePath, state); err != nil {
		return Repository{}, err
	}
	return result, nil
}

func writeMarkerLocked(dir, id string) error {
	dir, err := validateStateDir(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	unlock, err := lockFile(markerLockPath(dir))
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	if existing, err := ReadRepositoryID(dir); err == nil {
		if existing != id {
			return fmt.Errorf("repository marker belongs to %q", existing)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return persistMarker(RepositoryMarkerPath(dir), []byte(id+"\n"))
}
