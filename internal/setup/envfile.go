package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultEnvFiles is the default list of env files to handle.
var DefaultEnvFiles = []string{".env", ".env.local", ".env.development", ".env.development.local"}

// EnvFileHandler handles env file operations with injectable functions.
type EnvFileHandler struct {
	Symlink  func(oldname, newname string) error
	CopyFile func(src, dst string) error
}

// NewEnvFileHandler creates an EnvFileHandler with real OS functions.
func NewEnvFileHandler() *EnvFileHandler {
	return &EnvFileHandler{
		Symlink:  os.Symlink,
		CopyFile: copyFile,
	}
}

// HandleEnvFiles processes env files according to the strategy.
func (h *EnvFileHandler) HandleEnvFiles(mainDir, targetDir, strategy string, files []string) error {
	if strategy == "none" || strategy == "" {
		return nil
	}

	if len(files) == 0 {
		files = DefaultEnvFiles
	}

	for _, f := range files {
		src := filepath.Join(mainDir, f)
		if _, err := os.Stat(src); err != nil {
			continue // skip files that don't exist in main
		}

		dst := filepath.Join(targetDir, f)

		switch strategy {
		case "symlink":
			rel, err := filepath.Rel(targetDir, src)
			if err != nil {
				return fmt.Errorf("computing relative path for %s: %w", f, err)
			}
			if err := h.Symlink(rel, dst); err != nil {
				return fmt.Errorf("symlinking %s: %w", f, err)
			}

		case "copy":
			if err := h.CopyFile(src, dst); err != nil {
				return fmt.Errorf("copying %s: %w", f, err)
			}

		default:
			return fmt.Errorf("unknown env strategy: %q", strategy)
		}
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stating source: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	return nil
}
