//go:build !windows

package identity

import (
	"fmt"
	"os"
	"path/filepath"
)

// persistState uses same-directory replacement and fsyncs both file and directory.
func persistState(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state.json-*")
	if err != nil {
		return fmt.Errorf("creating identity state temporary file: %w", err)
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
		return fmt.Errorf("writing identity state: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing identity state: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening identity directory: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("syncing identity directory: %w", err)
	}
	return nil
}
