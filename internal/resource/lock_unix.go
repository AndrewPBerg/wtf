//go:build !windows

package resource

import (
	"fmt"
	"os"
	"syscall"
)

func lockResourceFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening resource lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("locking resource registry: %w", err)
	}
	return func() error {
		err1 := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		err2 := f.Close()
		if err1 != nil {
			return err1
		}
		return err2
	}, nil
}
