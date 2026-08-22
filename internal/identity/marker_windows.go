//go:build windows

package identity

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
	"path/filepath"
)

func persistMarker(path string, data []byte) error {
	d := filepath.Dir(path)
	f, e := os.CreateTemp(d, ".repository-id-*")
	if e != nil {
		return fmt.Errorf("creating repository marker: %w", e)
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	a, err := windows.UTF16PtrFromString(n)
	if err != nil {
		return fmt.Errorf("converting temporary marker path to UTF-16: %w", err)
	}
	b, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("converting marker path to UTF-16: %w", err)
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
