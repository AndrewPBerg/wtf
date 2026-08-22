package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store is a filesystem-backed identity store. A Store has no process-global state.
type Store struct{ home, statePath, lockPath string }

// NewStore creates a store rooted at home. An empty home uses WTF_HOME or ~/.wtf.
func NewStore(home string) (*Store, error) {
	if home == "" {
		home = os.Getenv("WTF_HOME")
		if home == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("finding home directory: %w", err)
			}
			home = filepath.Join(h, ".wtf")
		}
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("resolving WTF home: %w", err)
	}
	return &Store{home: home, statePath: filepath.Join(home, "state.json"), lockPath: filepath.Join(home, "state.lock")}, nil
}

// DefaultStore opens the WTF_HOME-aware global store.
func DefaultStore() (*Store, error) { return NewStore("") }

// Open is a concise alias for NewStore.
func Open(home string) (*Store, error) { return NewStore(home) }

// Paths returns the authoritative state and lock paths.
func (s *Store) Paths() (string, string) { return s.statePath, s.lockPath }

func emptyState() State {
	return State{Version: StateVersion, Repositories: []Repository{}, Workspaces: []Workspace{}}
}

func readState(path string) (State, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("reading identity state: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var state State
	if err := dec.Decode(&state); err != nil {
		return State{}, fmt.Errorf("decoding identity state: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return State{}, fmt.Errorf("decoding identity state: trailing data")
		}
		return State{}, fmt.Errorf("decoding identity state: trailing data: %w", err)
	}
	if err := state.Validate(); err != nil {
		return State{}, fmt.Errorf("validating identity state: %w", err)
	}
	return state, nil
}

// Load strictly reads the complete state. It never writes or repairs it.
func (s *Store) Load() (State, error) { return readState(s.statePath) }

// LookupRepository finds one repository by UUID or canonical locator.
func (s *Store) LookupRepository(query string) (Repository, error) {
	state, err := s.Load()
	if err != nil {
		return Repository{}, err
	}
	for _, repo := range state.Repositories {
		if repo.ID == query {
			return repo, nil
		}
	}
	canonical, pathErr := canonicalPath(query, "repository locator")
	var found []Repository
	for _, repo := range state.Repositories {
		if repo.LifecycleState == Active && pathErr == nil && repo.Locator == canonical {
			found = append(found, repo)
		}
	}
	if len(found) == 0 {
		return Repository{}, fmt.Errorf("repository %q not found", query)
	}
	if len(found) != 1 {
		return Repository{}, fmt.Errorf("repository %q is ambiguous", query)
	}
	return found[0], nil
}

// FindRepository is the query-oriented alias for LookupRepository.
func (s *Store) FindRepository(query string) (Repository, error) {
	return s.LookupRepository(query)
}

// LookupWorkspace finds one workspace by UUID, canonical name, or path.
func (s *Store) LookupWorkspace(query string) (Workspace, error) {
	state, err := s.Load()
	if err != nil {
		return Workspace{}, err
	}
	// UUIDs address retained tombstones. Human selectors address only
	// claim-retaining states, so reusing a removed name/path is not ambiguous.
	for _, w := range state.Workspaces {
		if w.ID == query {
			return w, nil
		}
	}
	canonical, pathErr := canonicalPath(query, "workspace path")
	var found []Workspace
	for _, w := range state.Workspaces {
		if (w.LifecycleState == Pending || w.LifecycleState == Active || w.LifecycleState == CleanupFailed) &&
			(w.Name == query || (pathErr == nil && w.Path == canonical)) {
			found = append(found, w)
		}
	}
	if len(found) == 0 {
		return Workspace{}, fmt.Errorf("workspace %q not found", query)
	}
	if len(found) != 1 {
		return Workspace{}, fmt.Errorf("workspace %q is ambiguous", query)
	}
	return found[0], nil
}

// FindWorkspace is an explicit alias for callers that prefer query terminology.
func (s *Store) FindWorkspace(query string) (Workspace, error) { return s.LookupWorkspace(query) }

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) mutate(fn func(*State) error) error {
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return fmt.Errorf("creating identity directory: %w", err)
	}
	unlock, err := lockFile(s.lockPath)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	state, err := readState(s.statePath)
	if err != nil {
		return err
	}
	if err := fn(&state); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("validating identity mutation: %w", err)
	}
	return writeState(s.statePath, state)
}

func writeState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding identity state: %w", err)
	}
	return persistState(path, append(data, '\n'))
}

// CreateRepository allocates and persists a repository identity.
func (s *Store) CreateRepository(locator string) (Repository, error) {
	var out Repository
	err := s.mutate(func(state *State) error {
		canonical, err := canonicalPath(locator, "repository locator")
		if err != nil {
			return err
		}
		for _, r := range state.Repositories {
			if r.LifecycleState == Active && r.Locator == canonical {
				return fmt.Errorf("active repository locator already exists")
			}
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		t := now()
		out = Repository{ID: id, Locator: canonical, LifecycleState: Active, CreatedAt: t, UpdatedAt: t}
		state.Repositories = append(state.Repositories, out)
		return nil
	})
	return out, err
}

// CreateWorkspace records a pending workspace and immediately claims its name and path.
func (s *Store) CreateWorkspace(repositoryID, name, backend, nativeName, path string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		if err := ValidateID(repositoryID); err != nil {
			return err
		}
		if err := ValidateName(name); err != nil {
			return err
		}
		canonical, err := canonicalPath(path, "workspace path")
		if err != nil {
			return err
		}
		if !validBackend(backend) || nativeName == "" {
			return fmt.Errorf("workspace backend and native name are required")
		}
		found := false
		for _, r := range state.Repositories {
			if r.ID == repositoryID {
				if r.LifecycleState != Active {
					return fmt.Errorf("repository %q is not active", repositoryID)
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("repository %q does not exist", repositoryID)
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		for _, other := range state.Workspaces {
			if other.LifecycleState != Removed && (other.Name == name || other.Path == canonical) {
				return fmt.Errorf("workspace name or path is already claimed")
			}
		}
		t := now()
		out = Workspace{ID: id, RepositoryID: repositoryID, Name: name, Backend: backend, NativeName: nativeName, Path: canonical, LifecycleState: Pending, CreatedAt: t, UpdatedAt: t}
		state.Workspaces = append(state.Workspaces, out)
		return nil
	})
	return out, err
}

// ActivateWorkspace performs the global name and path claims atomically.
func (s *Store) ActivateWorkspace(id string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState != Pending {
					return fmt.Errorf("workspace %q is not pending", id)
				}
				w.LifecycleState = Active
				w.UpdatedAt = now()
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// MarkCleanupFailed leaves a repairable tombstone after physical cleanup fails.
func (s *Store) MarkCleanupFailed(id string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState == Removed || w.LifecycleState == CleanupFailed {
					return fmt.Errorf("workspace %q is already removed", id)
				}
				w.LifecycleState = CleanupFailed
				w.CleanupFailedAt = now()
				w.UpdatedAt = w.CleanupFailedAt
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// FinalizeCleanup records successful external physical cleanup for a failed removal.
func (s *Store) FinalizeCleanup(id string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState != CleanupFailed {
					return fmt.Errorf("workspace %q is not cleanup_failed", id)
				}
				w.LifecycleState, w.RemovedAt, w.UpdatedAt = Removed, now(), now()
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// RemoveWorkspace leaves a tombstone after successful physical cleanup.
func (s *Store) RemoveWorkspace(id string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState == Removed || w.LifecycleState == CleanupFailed {
					return fmt.Errorf("workspace %q is immutable", id)
				}
				w.LifecycleState = Removed
				w.RemovedAt, w.UpdatedAt = now(), now()
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// RenameWorkspace changes a workspace's canonical name without changing its ID.
func (s *Store) RenameWorkspace(id, name string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		if err := ValidateName(name); err != nil {
			return err
		}
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState == Removed || w.LifecycleState == CleanupFailed {
					return fmt.Errorf("workspace %q is immutable", id)
				}
				if w.LifecycleState == Pending || w.LifecycleState == Active {
					for _, x := range state.Workspaces {
						if x.ID != id && x.LifecycleState != Removed && x.Name == name {
							return fmt.Errorf("active workspace name already exists")
						}
					}
				}
				w.Name = name
				if w.Backend == "jj" {
					w.NativeName = name
				}
				w.UpdatedAt = now()
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// MoveWorkspace changes a workspace path without changing its ID.
func (s *Store) MoveWorkspace(id, path string) (Workspace, error) {
	var out Workspace
	err := s.mutate(func(state *State) error {
		canonical, err := canonicalPath(path, "workspace path")
		if err != nil {
			return err
		}
		for i := range state.Workspaces {
			w := &state.Workspaces[i]
			if w.ID == id {
				if w.LifecycleState == Removed || w.LifecycleState == CleanupFailed {
					return fmt.Errorf("workspace %q is immutable", id)
				}
				if w.LifecycleState == Pending || w.LifecycleState == Active {
					for _, x := range state.Workspaces {
						if x.ID != id && x.LifecycleState != Removed && x.Path == canonical {
							return fmt.Errorf("active workspace path already exists")
						}
					}
				}
				w.Path = canonical
				w.UpdatedAt = now()
				out = *w
				return nil
			}
		}
		return fmt.Errorf("workspace %q not found", id)
	})
	return out, err
}

// AdoptionStatus describes whether an existing physical workspace is safe to
// bring under identity management.
type AdoptionStatus string

var (
	errAdoptionRenameRequired = errors.New("workspace adoption requires rename")
	errAdoptionRepairRequired = errors.New("workspace adoption requires repair")
)

const (
	// Adopted indicates that the physical workspace was safely recorded.
	Adopted AdoptionStatus = "adopted"
	// RenameRequired indicates that the legacy workspace needs explicit renaming.
	RenameRequired AdoptionStatus = "rename_required"
	// RepairRequired indicates that an exact pending or cleanup-failed workspace
	// must be repaired before it can be adopted. The existing workspace is
	// returned for diagnostics and the state is not changed.
	RepairRequired AdoptionStatus = "repair_required"
)

// AdoptionResult is non-mutating for RenameRequired and RepairRequired results.
type AdoptionResult struct {
	Status    AdoptionStatus
	Workspace Workspace
}

// AdoptWorkspace adopts an existing workspace only when all identity fields
// are canonical and globally available. Legacy entries are reported, never
// renamed or persisted.
func (s *Store) AdoptWorkspace(repositoryID, name, backend, nativeName, path string) (AdoptionResult, error) {
	var result AdoptionResult
	if err := ValidateName(name); err != nil {
		result.Status = RenameRequired
		return result, nil
	}
	canonical, err := canonicalPath(path, "workspace path")
	if err != nil {
		result.Status = RenameRequired
		return result, nil
	}
	if !validBackend(backend) || nativeName == "" || strings.ContainsAny(nativeName, "\x00\r\n") || (backend == string(JJ) && nativeName != name) {
		result.Status = RenameRequired
		return result, nil
	}
	err = s.mutate(func(state *State) error {
		var repo *Repository
		for i := range state.Repositories {
			if state.Repositories[i].ID == repositoryID {
				repo = &state.Repositories[i]
				break
			}
		}
		if repo == nil || repo.LifecycleState != Active {
			return fmt.Errorf("repository %q does not exist", repositoryID)
		}
		for _, w := range state.Workspaces {
			if w.LifecycleState == Removed {
				continue
			}
			exact := w.RepositoryID == repositoryID && w.Name == name && w.Backend == backend && w.NativeName == nativeName && w.Path == canonical
			if exact {
				result.Workspace = w
				switch w.LifecycleState {
				case Active:
					result.Status = Adopted
					return nil
				case Pending, CleanupFailed:
					result.Status = RepairRequired
					return errAdoptionRepairRequired
				}
			}
			if w.Name == name || w.Path == canonical || (w.RepositoryID == repositoryID && w.Backend == backend && w.NativeName == nativeName) {
				result.Status = RenameRequired
				return errAdoptionRenameRequired
			}
		}
		id, e := NewID()
		if e != nil {
			return e
		}
		t := now()
		result.Workspace = Workspace{ID: id, RepositoryID: repositoryID, Name: name, Backend: backend, NativeName: nativeName, Path: canonical, LifecycleState: Active, CreatedAt: t, UpdatedAt: t}
		state.Workspaces = append(state.Workspaces, result.Workspace)
		result.Status = Adopted
		return nil
	})
	if errors.Is(err, errAdoptionRenameRequired) || errors.Is(err, errAdoptionRepairRequired) {
		return result, nil
	}
	if err != nil {
		return AdoptionResult{}, err
	}
	return result, nil
}

// AdoptExistingWorkspace is a descriptive alias for AdoptWorkspace.
func (s *Store) AdoptExistingWorkspace(repositoryID, name, backend, nativeName, path string) (AdoptionResult, error) {
	return s.AdoptWorkspace(repositoryID, name, backend, nativeName, path)
}
