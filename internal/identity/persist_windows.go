//go:build windows

package identity

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// persistState uses the Windows kernel replacement primitive. Directory fsync
// is intentionally not attempted: Windows has no portable directory handle
// sync contract, while WRITE_THROUGH provides the supported runtime guarantee.
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
	from, err := windows.UTF16PtrFromString(tmpName)
	if err != nil {
		return fmt.Errorf("converting temporary identity state path to UTF-16: %w", err)
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("converting identity state path to UTF-16: %w", err)
	}
	if err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("replacing identity state: %w", err)
	}
	return nil
}
