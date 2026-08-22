//go:build !windows

package identity

import (
	"fmt"
	"os"
	"path/filepath"
)

func persistMarker(path string, data []byte) error {
	d := filepath.Dir(path)
	f, e := os.CreateTemp(d, ".repository-id-*")
	if e != nil {
		return fmt.Errorf("creating repository marker: %w", e)
	}
	n := f.Name()
	defer func() { _ = os.Remove(n) }()
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
	if e = os.Rename(n, path); e != nil {
		return e
	}
	x, e := os.Open(d)
	if e == nil {
		defer func() { _ = x.Close() }()
		e = x.Sync()
	}
	return e
}
