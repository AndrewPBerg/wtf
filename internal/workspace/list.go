package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Register records a workspace root for global instance discovery.
func Register(root string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".wtf", "workspaces.json")
	data, err := os.ReadFile(path)
	roots := []string{}
	if err == nil {
		if err := json.Unmarshal(data, &roots); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, existing := range roots {
		if existing == root {
			return nil
		}
	}
	roots = append(roots, root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(roots, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// Registered returns workspace roots previously seen by WTF.
func Registered() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".wtf", "workspaces.json"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var roots []string
	if err := json.Unmarshal(data, &roots); err != nil {
		return nil, err
	}
	return roots, nil
}

// List returns every persisted instance for a workspace.
func List(root string) ([]RuntimeState, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".wtf", "state", "instances"))
	if os.IsNotExist(err) {
		return []RuntimeState{}, nil
	}
	if err != nil {
		return nil, err
	}
	states := make([]RuntimeState, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, ".wtf", "state", "instances", entry.Name()))
		if err != nil {
			return nil, err
		}
		var state RuntimeState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Instance < states[j].Instance })
	return states, nil
}
