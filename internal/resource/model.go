// Package resource provides the versioned, workspace-owned resource lifecycle
// substrate. It stores declarations and metadata only; it never stores file
// contents or secret values.
package resource

import (
	"fmt"
	"sort"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/identity"
)

// Version is the resource registry schema version.
const Version = 1

// Kind identifies a supported resource type.
type Kind string

const (
	// KindFile identifies a declared file resource.
	KindFile Kind = "file"
	// KindPort identifies a declared port resource.
	KindPort Kind = "port"
)

// FileIntent is metadata-only desired state for a managed file.
type FileIntent struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode"`
	Secret bool   `json:"secret"`
}

// PortIntent is desired state for a named port lease.
type PortIntent struct {
	Name      string `json:"name"`
	Preferred int    `json:"preferred"`
}

// Desired is the manifest-derived resource intent for one workspace.
type Desired struct {
	Files []FileIntent `json:"files,omitempty"`
	Ports []PortIntent `json:"ports,omitempty"`
}

// FromManifest carries typed manifest intent into the lifecycle registry. It
// copies metadata only; it does not read any declared paths.
func FromManifest(m *config.Manifest) (Desired, error) {
	if m == nil || m.Version != Version {
		return Desired{}, fmt.Errorf("manifest version must be %d", Version)
	}
	d := Desired{}
	for _, f := range m.Resources.Files {
		d.Files = append(d.Files, FileIntent{Name: f.Name, Source: f.Source, Target: f.Target, Mode: f.Mode, Secret: f.Secret})
	}
	for _, p := range m.Resources.Ports {
		d.Ports = append(d.Ports, PortIntent{Name: p.Name, Preferred: p.Preferred})
	}
	if err := validateDesired(d); err != nil {
		return Desired{}, err
	}
	return d, nil
}

// ObservedState describes the latest metadata-only resource observation.
type ObservedState string

const (
	// ObservedAbsent means the resource target is missing.
	ObservedAbsent ObservedState = "absent"
	// ObservedPresent means the resource target matches the declaration.
	ObservedPresent ObservedState = "present"
	// ObservedInvalid means the resource target has drifted.
	ObservedInvalid ObservedState = "invalid"
	// ObservedUnknown means WTF could not observe the resource safely.
	ObservedUnknown ObservedState = "unknown"
)

// Observed is metadata-only physical state for one resource.
type Observed struct {
	Kind   Kind          `json:"kind"`
	Name   string        `json:"name"`
	State  ObservedState `json:"state"`
	Detail string        `json:"detail,omitempty"`
}

// LeaseState describes ownership of a resource lease.
type LeaseState string

const (
	// LeaseReleased means a prior lease is no longer held.
	LeaseReleased LeaseState = "released"
	// LeaseAcquired means the resource is currently owned by its workspace.
	LeaseAcquired LeaseState = "acquired"
)

// Lease records a UUID-owned resource lease.
type Lease struct {
	Kind  Kind       `json:"kind"`
	Name  string     `json:"name"`
	ID    string     `json:"id"`
	State LeaseState `json:"state"`
}

// LifecycleState describes the durable resource lifecycle.
type LifecycleState string

const (
	// LifecycleActive means resources may be reconciled.
	LifecycleActive LifecycleState = "active"
	// LifecycleCleanupFailed means repairable cleanup debt remains.
	LifecycleCleanupFailed LifecycleState = "cleanup_failed"
	// LifecycleFinalized means all workspace resource cleanup completed.
	LifecycleFinalized LifecycleState = "finalized"
)

// CleanupDebt records one resource cleanup failure.
type CleanupDebt struct {
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// FileOwnership records the metadata required to safely remove a managed file.
type FileOwnership struct {
	Name     string `json:"name"`
	Target   string `json:"target"`
	Mode     string `json:"mode"`
	Checksum string `json:"checksum,omitempty"`
}

// Workspace is all durable resource state owned by one canonical workspace UUID.
type Workspace struct {
	Version       int             `json:"version"`
	WorkspaceID   string          `json:"workspace_id"`
	Desired       Desired         `json:"desired"`
	Observed      []Observed      `json:"observed,omitempty"`
	Leases        []Lease         `json:"leases,omitempty"`
	FileOwnership []FileOwnership `json:"file_ownership,omitempty"`
	CleanupDebt   []CleanupDebt   `json:"cleanup_debt,omitempty"`
	Lifecycle     LifecycleState  `json:"lifecycle"`
}

func validateWorkspaceID(id string) error {
	if err := identity.ValidateID(id); err != nil {
		return fmt.Errorf("workspace ID: %w", err)
	}
	return nil
}

func validateKindName(kind Kind, name string) error {
	if kind != KindFile && kind != KindPort {
		return fmt.Errorf("unsupported resource kind %q", kind)
	}
	if name == "" {
		return fmt.Errorf("resource name must not be empty")
	}
	return nil
}

func validateDesired(d Desired) error {
	seen := make(map[string]struct{}, len(d.Files)+len(d.Ports))
	for _, f := range d.Files {
		if err := validateKindName(KindFile, f.Name); err != nil {
			return err
		}
		if f.Source == "" || f.Target == "" {
			return fmt.Errorf("file %q requires source and target", f.Name)
		}
		if f.Mode != "symlink" && f.Mode != "copy" {
			return fmt.Errorf("file %q has invalid mode", f.Name)
		}
		key := string(KindFile) + "\x00" + f.Name
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate resource %q", f.Name)
		}
		seen[key] = struct{}{}
	}
	for _, p := range d.Ports {
		if err := validateKindName(KindPort, p.Name); err != nil {
			return err
		}
		if p.Preferred < 1 || p.Preferred > 65535 {
			return fmt.Errorf("port %q is out of range", p.Name)
		}
		key := string(KindPort) + "\x00" + p.Name
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate resource %q", p.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func sortObserved(v []Observed) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Kind != v[j].Kind {
			return v[i].Kind < v[j].Kind
		}
		return v[i].Name < v[j].Name
	})
}
func sortLeases(v []Lease) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Kind != v[j].Kind {
			return v[i].Kind < v[j].Kind
		}
		return v[i].Name < v[j].Name
	})
}
func sortDebt(v []CleanupDebt) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Kind != v[j].Kind {
			return v[i].Kind < v[j].Kind
		}
		return v[i].Name < v[j].Name
	})
}
