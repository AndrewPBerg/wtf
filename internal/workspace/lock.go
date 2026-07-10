package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockTimeout = 10 * time.Second

// withLock serializes lifecycle operations across WTF processes for one
// workspace. A directory is created atomically on every supported platform.
func withLock(root string, fn func() error) error {
	path := filepath.Join(root, ".wtf", "state", "lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating workspace lock directory: %w", err)
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		err := os.Mkdir(path, 0o700)
		if err == nil {
			defer func() { _ = os.Remove(path) }()
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquiring workspace lock: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("workspace is busy: timed out waiting for %s", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
