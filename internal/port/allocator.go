package port

import "fmt"

// Allocator manages port assignments for worktree branches.
type Allocator struct {
	base  int
	store Store
}

// New creates an Allocator with the given base port and persistence store.
func New(base int, store Store) *Allocator {
	return &Allocator{base: base, store: store}
}

// Allocate returns a port for branch. If branch already has a port, it is
// returned unchanged. Otherwise the lowest available port starting from
// base is assigned and persisted.
func (a *Allocator) Allocate(branch string) (int, error) {
	m, err := a.store.Load()
	if err != nil {
		return 0, fmt.Errorf("loading ports: %w", err)
	}

	if p, ok := m[branch]; ok {
		return p, nil
	}

	used := make(map[int]bool, len(m))
	for _, p := range m {
		used[p] = true
	}

	port := a.base
	for used[port] {
		port++
	}

	m[branch] = port
	if err := a.store.Save(m); err != nil {
		return 0, fmt.Errorf("saving ports: %w", err)
	}
	return port, nil
}

// Lookup returns the port assigned to branch without allocating.
// The bool is false if branch has no assignment.
func (a *Allocator) Lookup(branch string) (int, bool, error) {
	m, err := a.store.Load()
	if err != nil {
		return 0, false, fmt.Errorf("loading ports: %w", err)
	}
	p, ok := m[branch]
	return p, ok, nil
}

// Release removes the port assignment for branch.
func (a *Allocator) Release(branch string) error {
	m, err := a.store.Load()
	if err != nil {
		return fmt.Errorf("loading ports: %w", err)
	}

	if _, ok := m[branch]; !ok {
		return nil // nothing to release
	}

	delete(m, branch)
	if err := a.store.Save(m); err != nil {
		return fmt.Errorf("saving ports: %w", err)
	}
	return nil
}

// ListAll returns all current branch→port assignments.
func (a *Allocator) ListAll() (map[string]int, error) {
	m, err := a.store.Load()
	if err != nil {
		return nil, fmt.Errorf("loading ports: %w", err)
	}
	return m, nil
}
