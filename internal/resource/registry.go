package resource

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Registry is a concurrency-safe, versioned resource registry. Each mutation
// is serialized and committed with a same-directory rename.
type Registry struct {
	mu   sync.Mutex
	path string
}

// NewRegistry returns a registry backed by path. The legacy port store is not
// read or migrated.
func NewRegistry(path string) *Registry { return &Registry{path: path} }

type registryFile struct {
	Version    int                  `json:"version"`
	Workspaces map[string]Workspace `json:"workspaces"`
}

// Load reads resource state without creating or repairing registry files.
func (r *Registry) Load() (map[string]Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

func (r *Registry) lock() (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return nil, fmt.Errorf("creating resource registry directory: %w", err)
	}
	return lockResourceFile(r.path + ".lock")
}

func (r *Registry) load() (map[string]Workspace, error) {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return map[string]Workspace{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading resource registry: %w", err)
	}
	var raw registryFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding resource registry: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("decoding resource registry: trailing data")
	}
	if raw.Version != Version {
		return nil, fmt.Errorf("resource registry version must be %d", Version)
	}
	if raw.Workspaces == nil {
		return nil, fmt.Errorf("resource registry workspaces is required")
	}
	for id, w := range raw.Workspaces {
		if err := validateWorkspaceID(id); err != nil {
			return nil, err
		}
		if w.Version != Version || w.WorkspaceID != id {
			return nil, fmt.Errorf("invalid workspace record %q", id)
		}
		if err := validateDesired(w.Desired); err != nil {
			return nil, err
		}
		if w.Lifecycle != LifecycleActive && w.Lifecycle != LifecycleCleanupFailed && w.Lifecycle != LifecycleFinalized {
			return nil, fmt.Errorf("invalid lifecycle for workspace %q", id)
		}
		for _, o := range w.Observed {
			if err := validateKindName(o.Kind, o.Name); err != nil {
				return nil, err
			}
			if o.State != ObservedAbsent && o.State != ObservedPresent && o.State != ObservedInvalid && o.State != ObservedUnknown {
				return nil, fmt.Errorf("invalid observed state %q", o.State)
			}
		}
		for _, l := range w.Leases {
			if err := validateKindName(l.Kind, l.Name); err != nil {
				return nil, err
			}
			if l.ID == "" || (l.State != LeaseAcquired && l.State != LeaseReleased) {
				return nil, fmt.Errorf("invalid lease for workspace %q", id)
			}
		}
		for _, f := range w.FileOwnership {
			if f.Name == "" || f.Target == "" || (f.Mode != "symlink" && f.Mode != "copy") {
				return nil, fmt.Errorf("invalid file ownership for workspace %q", id)
			}
			if f.Mode == "copy" && f.Checksum == "" {
				return nil, fmt.Errorf("copy ownership for %q lacks a checksum", f.Name)
			}
		}
		for _, d := range w.CleanupDebt {
			if err := validateKindName(d.Kind, d.Name); err != nil {
				return nil, err
			}
			if d.Reason == "" {
				return nil, fmt.Errorf("cleanup debt reason must not be empty")
			}
		}
	}
	return raw.Workspaces, nil
}

func (r *Registry) save(workspaces map[string]Workspace) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("creating resource registry directory: %w", err)
	}
	raw := registryFile{Version: Version, Workspaces: workspaces}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding resource registry: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".resource-registry-*")
	if err != nil {
		return fmt.Errorf("creating resource registry temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("writing resource registry: %w", err)
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		return fmt.Errorf("committing resource registry: %w", err)
	}
	return nil
}

func (r *Registry) update(fn func(map[string]Workspace) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	ws, err := r.load()
	if err != nil {
		return err
	}
	if err := fn(ws); err != nil {
		return err
	}
	return r.save(ws)
}

// SetDesired atomically records manifest intent for a canonical workspace.
func (r *Registry) SetDesired(id string, desired Desired) error {
	if err := validateWorkspaceID(id); err != nil {
		return err
	}
	if err := validateDesired(desired); err != nil {
		return err
	}
	return r.update(func(all map[string]Workspace) error {
		w := all[id]
		if w.WorkspaceID == "" {
			w = Workspace{Version: Version, WorkspaceID: id, Lifecycle: LifecycleActive}
		}
		if w.Lifecycle == LifecycleFinalized {
			return fmt.Errorf("workspace %q is finalized", id)
		}
		w.Desired = desired
		w.Version = Version
		w.WorkspaceID = id
		if w.Lifecycle == "" {
			w.Lifecycle = LifecycleActive
		}
		all[id] = w
		return nil
	})
}

// RecordFileOwnership persists metadata proving WTF may later remove a managed file.
func (r *Registry) RecordFileOwnership(id string, ownership FileOwnership) error {
	if ownership.Name == "" || ownership.Target == "" || (ownership.Mode != "symlink" && ownership.Mode != "copy") {
		return fmt.Errorf("invalid file ownership")
	}
	if ownership.Mode == "copy" && ownership.Checksum == "" {
		return fmt.Errorf("copy ownership requires a checksum")
	}
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		for i := range w.FileOwnership {
			if w.FileOwnership[i].Name == ownership.Name {
				w.FileOwnership[i] = ownership
				all[id] = w
				return nil
			}
		}
		w.FileOwnership = append(w.FileOwnership, ownership)
		all[id] = w
		return nil
	})
}

// RemoveFileOwnership removes durable metadata after safe file cleanup.
func (r *Registry) RemoveFileOwnership(id, name string) error {
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		for i, f := range w.FileOwnership {
			if f.Name == name {
				w.FileOwnership = append(w.FileOwnership[:i], w.FileOwnership[i+1:]...)
				all[id] = w
				return nil
			}
		}
		return fmt.Errorf("file ownership %q not found", name)
	})
}

// Get returns one workspace record selected by canonical workspace UUID.
func (r *Registry) Get(id string) (Workspace, error) {
	if err := validateWorkspaceID(id); err != nil {
		return Workspace{}, err
	}
	all, err := r.Load()
	if err != nil {
		return Workspace{}, err
	}
	w, ok := all[id]
	if !ok {
		return Workspace{}, fmt.Errorf("workspace %q not found", id)
	}
	return w, nil
}

func newLeaseID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("creating lease ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Acquire atomically acquires a declared resource lease.
func (r *Registry) Acquire(id string, kind Kind, name string) (Lease, error) {
	if err := validateWorkspaceID(id); err != nil {
		return Lease{}, err
	}
	if err := validateKindName(kind, name); err != nil {
		return Lease{}, err
	}
	var result Lease
	err := r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		if !declared(w.Desired, kind, name) {
			return fmt.Errorf("resource %s/%s is not desired", kind, name)
		}
		for _, l := range w.Leases {
			if l.Kind == kind && l.Name == name && l.State == LeaseAcquired {
				return fmt.Errorf("resource %s/%s is already acquired", kind, name)
			}
		}
		leaseID, err := newLeaseID()
		if err != nil {
			return err
		}
		result = Lease{Kind: kind, Name: name, ID: leaseID, State: LeaseAcquired}
		w.Leases = append(w.Leases, result)
		sortLeases(w.Leases)
		all[id] = w
		return nil
	})
	return result, err
}

// Release atomically releases a lease. An unknown lease is an error.
func (r *Registry) Release(id string, kind Kind, name, leaseID string) error {
	if err := validateWorkspaceID(id); err != nil {
		return err
	}
	if err := validateKindName(kind, name); err != nil {
		return err
	}
	return r.update(func(all map[string]Workspace) error {
		w, ok := all[id]
		if !ok {
			return fmt.Errorf("workspace %q not found", id)
		}
		for i := range w.Leases {
			if w.Leases[i].Kind == kind && w.Leases[i].Name == name && w.Leases[i].ID == leaseID {
				if w.Leases[i].State == LeaseReleased {
					return nil
				}
				w.Leases[i].State = LeaseReleased
				all[id] = w
				return nil
			}
		}
		return fmt.Errorf("lease %q not found", leaseID)
	})
}

func declared(d Desired, kind Kind, name string) bool {
	if kind == KindFile {
		for _, x := range d.Files {
			if x.Name == name {
				return true
			}
		}
	}
	if kind == KindPort {
		for _, x := range d.Ports {
			if x.Name == name {
				return true
			}
		}
	}
	return false
}
