package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// SetupResult reports scaffold files relative to the current directory.
type SetupResult struct {
	Path     string   `json:"path"`
	Created  []string `json:"created"`
	Existing []string `json:"existing"`
}

// Setup creates a local, ignored configuration scaffold without running hooks
// or registering a workspace. Existing files are never replaced.
func (a *App) Setup(ctx context.Context, schema []byte) (SetupResult, error) {
	root, err := filepath.Abs(a.Cwd)
	result := SetupResult{Path: filepath.Join(root, ".wtf"), Created: []string{}, Existing: []string{}}
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	for _, path := range []string{result.Path, filepath.Join(result.Path, "hooks"), filepath.Join(result.Path, "tasks")} {
		mkdirErr := os.Mkdir(path, 0755)
		if mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
			return result, mkdirErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return result, err
		}
		if !info.IsDir() {
			return result, Fail("invalid_setup_path", "Expected a directory, not a file or symlink: "+path)
		}
		if path != result.Path {
			name := ".wtf/" + filepath.Base(path) + "/"
			if mkdirErr == nil {
				result.Created = append(result.Created, name)
			} else {
				result.Existing = append(result.Existing, name)
			}
		}
	}
	files := []struct {
		name string
		data []byte
	}{
		{".gitignore", []byte("*\n")},
		{"schema.json", schema},
		{"config.toml", []byte("#:schema ./schema.json\n\nversion = 1\n")},
	}
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		path := filepath.Join(result.Path, filepath.FromSlash(file.name))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return result, statErr
			}
			if !info.Mode().IsRegular() {
				return result, Fail("invalid_setup_path", "Expected a regular file, not a directory or symlink: "+path)
			}
			result.Existing = append(result.Existing, ".wtf/"+file.name)
			continue
		}
		if err != nil {
			return result, err
		}
		_, writeErr := f.Write(file.data)
		closeErr := f.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return result, errors.Join(err, os.Remove(path))
		}
		result.Created = append(result.Created, ".wtf/"+file.name)
	}
	return result, nil
}

// SetupTeardownResult reports removal of local configuration only.
type SetupTeardownResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// TeardownSetup inspects the current directory's .wtf and removes it only after
// explicit confirmation. Root-scoped removal never follows symlinks outside it.
func (a *App) TeardownSetup(ctx context.Context, confirmed bool) (SetupTeardownResult, error) {
	path, err := filepath.Abs(a.Cwd)
	result := SetupTeardownResult{Path: filepath.Join(path, ".wtf"), Status: "confirmation_required"}
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(".wtf")
	if errors.Is(err, os.ErrNotExist) {
		result.Status = "not_configured"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, Fail("invalid_setup_path", "Expected a .wtf directory, not a file or symlink: "+result.Path)
	}
	if !confirmed {
		return result, nil
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = root.RemoveAll(".wtf"); err != nil {
		return result, err
	}
	result.Status = "removed"
	return result, nil
}
