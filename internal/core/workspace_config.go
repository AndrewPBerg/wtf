package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// inheritWorkspaceConfig transfers only project configuration, never local
// artifacts or host identity/state. It runs only for newly created workspaces.
func inheritWorkspaceConfig(ctx context.Context, sourcePath, destinationPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := os.OpenRoot(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	info, err := source.Lstat(".wtf")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return Fail("invalid_setup_path", "Source .wtf must be a directory")
	}
	dest, err := os.OpenRoot(destinationPath)
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()
	if err := dest.MkdirAll(".wtf", 0755); err != nil {
		return err
	}
	info, err = dest.Lstat(".wtf")
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return Fail("invalid_setup_path", "Destination .wtf must be a directory")
	}
	rt := &Runtime{Context: ctx, Repo: Repo{Root: sourcePath}}
	entries, bytes := 0, int64(0)
	var transfer func(string) error
	transfer = func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := source.Lstat(name)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		entries++
		if entries > fileEntryLimit {
			return Fail("file_limit", "Workspace configuration exceeds 1024 entries")
		}
		if info.IsDir() {
			if existing, err := dest.Lstat(name); err == nil && !existing.IsDir() {
				return Fail("invalid_setup_path", "Configuration directory conflicts with destination: "+name)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := mutationPath(dest, name); err != nil {
				return err
			}
			if err := dest.MkdirAll(name, info.Mode().Perm()); err != nil {
				return err
			}
			children, err := rt.listFiles(source, name)
			if err != nil {
				return err
			}
			for _, child := range children {
				if err := transfer(filepath.Join(name, child.Name)); err != nil {
					return err
				}
			}
			return nil
		}
		if info.Mode().IsRegular() {
			bytes += info.Size()
			if bytes > copyByteLimit {
				return Fail("file_limit", "Workspace configuration exceeds 32 MiB")
			}
		}
		_, err = rt.copyFile(source, name, dest, name, fileOptions{Replace: true, Symlinks: "preserve"})
		return err
	}
	for _, name := range []string{".gitignore", "schema.json", "config.toml", "AGENTS.md", guideChecksumFile, "hooks", "tasks"} {
		path := filepath.Join(".wtf", name)
		// Structural entries must be independent files/directories, not links.
		if info, err := source.Lstat(path); err == nil {
			isDirectory := name == "hooks" || name == "tasks"
			if isDirectory && !info.IsDir() || !isDirectory && !info.Mode().IsRegular() {
				return Fail("invalid_setup_path", "Expected a configuration file or directory: "+path)
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := transfer(path); err != nil {
			return err
		}
	}
	// Legacy explicit entrypoints may sit beside hooks/ rather than inside it.
	cfg, err := LoadConfig(sourcePath)
	if err != nil {
		return err
	}
	if cfg.Lua != "" && !strings.HasPrefix(filepath.ToSlash(filepath.Clean(cfg.Lua)), "hooks/") {
		if err := transfer(filepath.Join(".wtf", cfg.Lua)); err != nil {
			return err
		}
	}
	for _, name := range []string{"hooks", "tasks"} {
		path := filepath.Join(".wtf", name)
		if info, err := dest.Lstat(path); err == nil && !info.IsDir() {
			return Fail("invalid_setup_path", "Expected a configuration directory: "+path)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := dest.MkdirAll(path, 0755); err != nil {
			return err
		}
	}
	_, err = rt.writeFile(dest, ".wtf/.gitignore", []byte("*\n"), false)
	return err
}
