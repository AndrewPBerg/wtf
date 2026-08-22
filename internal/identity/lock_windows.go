//go:build windows

package identity

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile uses the kernel byte-range lock API. The lock is released by the
// kernel when the process exits, so a dead process cannot strand stale state.
func lockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening identity lock: %w", err)
	}
	h := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("locking identity state: %w", err)
	}
	return func() error {
		err1 := windows.UnlockFileEx(h, 0, 1, 0, &overlapped)
		err2 := f.Close()
		if err1 != nil {
			return err1
		}
		return err2
	}, nil
}
