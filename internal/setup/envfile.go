package setup

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// skipDirs are directories that should not be searched for env files.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".venv":        true,
	"vendor":       true,
	"__pycache__":  true,
	".next":        true,
	".nuxt":        true,
	"dist":         true,
	"build":        true,
}

// DiscoverEnvFiles walks rootDir and returns relative paths to all .env* files,
// including those in subdirectories (e.g. app/.env). Directories like
// node_modules, .git, and vendor are skipped.
func DiscoverEnvFiles(rootDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip dirs we can't read
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".env") {
			rel, relErr := filepath.Rel(rootDir, path)
			if relErr != nil {
				return nil
			}
			files = append(files, rel)
		}
		return nil
	})
	return files, err
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

		// Ensure parent directory exists (for subdirectory env files like app/.env)
		if dir := filepath.Dir(dst); dir != targetDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", f, err)
			}
		}

		// Handle existing file at target
		if info, lErr := os.Lstat(dst); lErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				// Already a symlink — check if it points to the right place
				rel, _ := filepath.Rel(filepath.Dir(dst), src)
				existing, _ := os.Readlink(dst)
				if existing == rel {
					continue // already correct
				}
			}
			// Remove existing file/symlink before creating new one
			if err := os.Remove(dst); err != nil {
				return fmt.Errorf("removing existing %s: %w", f, err)
			}
		}

		switch strategy {
		case "symlink":
			rel, err := filepath.Rel(filepath.Dir(dst), src)
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
