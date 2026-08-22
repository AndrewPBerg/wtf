// Package identity owns WTF's durable repository and workspace identities.
package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// StateVersion is the current on-disk identity schema version.
const StateVersion = 1

// LifecycleState is the durable state of an identity.
type LifecycleState string

const (
	// Pending reserves an identity while physical creation is in progress.
	Pending LifecycleState = "pending"
	// Active is a live, globally claimed workspace.
	Active LifecycleState = "active"
	// Removed is a retained tombstone.
	Removed LifecycleState = "removed"
	// CleanupFailed is a retained tombstone requiring repair.
	CleanupFailed LifecycleState = "cleanup_failed"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(/[a-z0-9][a-z0-9_-]*)+$`)

// NewID returns a canonical, random RFC 4122 version 4 UUID.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating UUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// NewUUID is an alias for NewID for callers that name the value by its format.
func NewUUID() (string, error) { return NewID() }

// ValidateID accepts only canonical lowercase RFC 4122 UUID text.
func ValidateID(id string) error {
	if !uuidPattern.MatchString(id) {
		return fmt.Errorf("invalid canonical UUID %q", id)
	}
	return nil
}

// ValidateUUID is an alias for ValidateID.
func ValidateUUID(id string) error { return ValidateID(id) }

func validTimestamp(value string) bool {
	if value == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validState(s LifecycleState) bool {
	return s == Pending || s == Active || s == Removed || s == CleanupFailed
}

// Repository is a durable repository identity.
type Repository struct {
	ID             string         `json:"id"`
	Locator        string         `json:"locator"`
	LifecycleState LifecycleState `json:"lifecycle_state"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

// Backend identifies the supported workspace implementations.
type Backend string

const (
	// Git is the Git worktree backend.
	Git Backend = "git"
	// JJ is the Jujutsu workspace backend.
	JJ Backend = "jj"
)

func validBackend(value string) bool { return Backend(value) == Git || Backend(value) == JJ }

// Workspace is a durable physical workspace identity.
type Workspace struct {
	ID              string         `json:"id"`
	RepositoryID    string         `json:"repository_id"`
	Name            string         `json:"name"`
	Backend         string         `json:"backend"`
	NativeName      string         `json:"native_name"`
	Path            string         `json:"path"`
	LifecycleState  LifecycleState `json:"lifecycle_state"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	RemovedAt       string         `json:"removed_at,omitempty"`
	CleanupFailedAt string         `json:"cleanup_failed_at,omitempty"`
}

// State is the complete versioned on-disk document.
type State struct {
	Version      int          `json:"version"`
	Repositories []Repository `json:"repositories"`
	Workspaces   []Workspace  `json:"workspaces"`
}

// ValidateName validates a canonical WTF name. It deliberately does not
// normalize input: callers must not accidentally claim a different name.
func ValidateName(name string) error {
	// Names are identifiers, not paths: keeping this grammar ASCII-only avoids
	// Unicode case folding, separator, and normalization ambiguities.
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid canonical workspace name %q", name)
	}
	return nil
}

// CanonicalPhysicalPath returns the stable local path form used by identity
// joins. It resolves symlink aliases where possible without requiring the
// target itself to exist.
func CanonicalPhysicalPath(value string) (string, error) {
	return canonicalPath(value, "path")
}

func canonicalPath(value, field string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "://") {
		return "", fmt.Errorf("%s must be a local absolute path", field)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", field, err)
	}
	absolute = filepath.Clean(absolute)
	// EvalSymlinks cannot resolve a path before it is created. Resolve the
	// deepest existing ancestor, then append the not-yet-created suffix.
	parts := []string{}
	probe := absolute
	for {
		if _, err := os.Lstat(probe); err == nil {
			resolved, err := filepath.EvalSymlinks(probe)
			if err != nil {
				return "", fmt.Errorf("resolving %s symlinks: %w", field, err)
			}
			for i := len(parts) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, parts[i])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("checking %s: %w", field, err)
		}
		next := filepath.Dir(probe)
		if next == probe {
			return "", fmt.Errorf("%s has no existing ancestor", field)
		}
		parts = append(parts, filepath.Base(probe))
		probe = next
	}
}

// Validate checks every record and all global invariants. It does not repair state.
func (s State) Validate() error {
	if s.Version != StateVersion {
		return fmt.Errorf("unsupported identity state version %d", s.Version)
	}
	if s.Repositories == nil || s.Workspaces == nil {
		return fmt.Errorf("identity state is missing repositories or workspaces")
	}
	ids := map[string]bool{}
	repos := map[string]bool{}
	locators := map[string]bool{}
	for i, r := range s.Repositories {
		if err := ValidateID(r.ID); err != nil {
			return fmt.Errorf("repository %d: %w", i, err)
		}
		if ids[r.ID] {
			return fmt.Errorf("duplicate identity %q", r.ID)
		}
		ids[r.ID] = true
		canonicalLocator, err := canonicalPath(r.Locator, "repository locator")
		if err != nil || canonicalLocator != r.Locator || !validState(r.LifecycleState) || !validTimestamp(r.CreatedAt) || !validTimestamp(r.UpdatedAt) {
			return fmt.Errorf("invalid repository %q", r.ID)
		}
		if r.LifecycleState == Active {
			if locators[r.Locator] {
				return fmt.Errorf("duplicate active repository locator %q", r.Locator)
			}
			locators[r.Locator] = true
		}
		if r.LifecycleState == Active {
			repos[r.ID] = true
		}
	}
	names, paths := map[string]bool{}, map[string]bool{}
	for i, w := range s.Workspaces {
		if err := ValidateID(w.ID); err != nil {
			return fmt.Errorf("workspace %d: %w", i, err)
		}
		if ids[w.ID] {
			return fmt.Errorf("duplicate identity %q", w.ID)
		}
		ids[w.ID] = true
		if !repos[w.RepositoryID] || !validBackend(w.Backend) || w.NativeName == "" || !validTimestamp(w.CreatedAt) || !validTimestamp(w.UpdatedAt) || !validState(w.LifecycleState) {
			return fmt.Errorf("invalid workspace %q", w.ID)
		}
		canonicalWorkspacePath, err := canonicalPath(w.Path, "workspace path")
		if err != nil || canonicalWorkspacePath != w.Path {
			return fmt.Errorf("invalid workspace %q: path is not physically canonical", w.ID)
		}
		if err := ValidateName(w.Name); err != nil {
			return err
		}
		switch w.LifecycleState {
		case Removed:
			if !validTimestamp(w.RemovedAt) || (w.CleanupFailedAt != "" && !validTimestamp(w.CleanupFailedAt)) {
				return fmt.Errorf("workspace %q has invalid removal history", w.ID)
			}
		case CleanupFailed:
			if !validTimestamp(w.CleanupFailedAt) || w.RemovedAt != "" {
				return fmt.Errorf("workspace %q has invalid cleanup failure state", w.ID)
			}
		default:
			if w.RemovedAt != "" || w.CleanupFailedAt != "" {
				return fmt.Errorf("workspace %q has removal history while live", w.ID)
			}
		}
		if w.LifecycleState == Pending || w.LifecycleState == Active || w.LifecycleState == CleanupFailed {
			if names[w.Name] {
				return fmt.Errorf("duplicate claimed workspace name %q", w.Name)
			}
			names[w.Name] = true
			p := w.Path
			if paths[p] {
				return fmt.Errorf("duplicate claimed workspace path %q", w.Path)
			}
			paths[p] = true
		}
	}
	return nil
}
