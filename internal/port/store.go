package port

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store abstracts port-assignment persistence.
type Store interface {
	Load() (map[string]int, error)
	Save(map[string]int) error
}

// FileStore persists port assignments as JSON.
type FileStore struct {
	path string
}

// NewFileStore creates a store backed by the given file path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load reads the port map from disk. Returns an empty map if the file
// does not exist.
func (s *FileStore) Load() (map[string]int, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]int{}, nil
		}
		return nil, fmt.Errorf("reading port store: %w", err)
	}

	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing port store: %w", err)
	}
	return m, nil
}

// Save writes the port map to disk, creating parent directories as needed.
func (s *FileStore) Save(m map[string]int) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating port store directory: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling port store: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("writing port store: %w", err)
	}
	return nil
}
