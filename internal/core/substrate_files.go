package core

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const fileByteLimit = 8 << 20
const copyByteLimit = 32 << 20
const fileEntryLimit = 1024

type fileOptions struct {
	Root, From, To, Symlinks   string
	Replace, Recursive, Follow bool
}

type fileOutcome struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}
type fileRecord struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	Target string `json:"target,omitempty"`
}

func (r *Runtime) fileRoot(scope string) (*os.Root, error) {
	if scope == "" {
		if r.Workspace.ID != "" {
			scope = "workspace"
		} else {
			scope = "project"
		}
	}
	var path string
	switch scope {
	case "project":
		path = r.Repo.Root
	case "workspace":
		if err := r.requireWorkspace(r.L); err != nil {
			return nil, err
		}
		path = r.Workspace.Path
	default:
		return nil, Fail("invalid_path", "Root must be project or workspace")
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fileError(err)
	}
	root, err := os.OpenRoot(actual)
	return root, fileError(err)
}

func fileName(path string, mutate bool) (string, error) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", Fail("invalid_path", "Expected a nonempty relative path")
	}
	path = filepath.Clean(path)
	if path == ".." || strings.HasPrefix(path, ".."+string(os.PathSeparator)) {
		return "", Fail("invalid_path", "Path escapes its selected root")
	}
	if mutate && (path == "." || protectedFile(path)) {
		return "", Fail("protected_path", "Cannot change a root, VCS metadata or the workspace identity marker")
	}
	return path, nil
}

func protectedFile(path string) bool {
	path = filepath.ToSlash(path)
	return path == ".git" || strings.HasPrefix(path, ".git/") || path == ".jj" || strings.HasPrefix(path, ".jj/") || path == ".wtf/workspace.json"
}

func mutationPath(root *os.Root, name string) error {
	// Resolve the existing parent prefix to protect metadata reached through aliases.
	parent := filepath.Join(root.Name(), filepath.Dir(name))
	var suffix []string
	for {
		actual, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				actual = filepath.Join(actual, suffix[i])
			}
			actual = filepath.Join(actual, filepath.Base(name))
			rel, err := filepath.Rel(root.Name(), actual)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return Fail("invalid_path", "Parent escapes the selected root")
			}
			if protectedFile(rel) {
				return Fail("protected_path", "Cannot change host-owned metadata")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fileError(err)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fileError(err)
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = next
	}
}

func (r *Runtime) removalPath(root *os.Root, name string, recursive bool) error {
	if err := mutationPath(root, name); err != nil {
		return err
	}
	if r.Repo.Store == "" {
		return nil
	}
	state, err := r.Repo.Load()
	if err != nil {
		return err
	}
	target := filepath.Join(root.Name(), name)
	// A final symlink is removed as a link, not as its resolved workspace target.
	info, err := root.Lstat(name)
	if err != nil {
		return fileError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if actual, err := filepath.EvalSymlinks(target); err == nil {
		target = actual
	}
	for _, workspace := range state.Workspaces {
		if workspace.Status == "removed" {
			continue
		}
		rel, err := filepath.Rel(target, workspace.Path)
		if err == nil && (rel == "." || recursive && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return Fail("protected_path", "Use workspace removal for a managed workspace or its containing directory")
		}
	}
	return nil
}

func fileError(err error) error {
	if err == nil {
		return nil
	}
	if ErrorCode(err) != "operation_failed" {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		return Fail("file_missing", "Requested file or directory is missing")
	}
	if errors.Is(err, os.ErrPermission) {
		return Fail("file_permission", "File operation was denied")
	}
	return Fail("file_operation_failed", err.Error())
}

// openRead follows links only into the two explicit context roots. Root.Open
// provides confinement even if the resolved path is changed concurrently.
func (r *Runtime) openRead(root *os.Root, name string) (*os.File, error) {
	f, err := root.OpenFile(name, rootReadFlags(), 0)
	if err == nil {
		return f, nil
	}
	actual, resolutionErr := filepath.EvalSymlinks(filepath.Join(root.Name(), name))
	if resolutionErr != nil {
		return nil, fileError(resolutionErr)
	}
	for _, scope := range []string{"project", "workspace"} {
		allowed, rootErr := r.fileRoot(scope)
		if rootErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(allowed.Name(), actual)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			f, openErr := allowed.OpenFile(rel, rootReadFlags(), 0)
			_ = allowed.Close()
			return f, fileError(openErr)
		}
		_ = allowed.Close()
	}
	return nil, Fail("invalid_path", "Symlink target escapes the project and workspace roots")
}

func (r *Runtime) readFile(root *os.Root, name string) ([]byte, error) {
	if err := r.Context.Err(); err != nil {
		return nil, err
	}
	f, err := r.openRead(root, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fileError(err)
	}
	if !info.Mode().IsRegular() {
		return nil, Fail("invalid_file_type", "Read requires a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, fileByteLimit+1))
	if err != nil {
		return nil, fileError(err)
	}
	if len(data) > fileByteLimit {
		return nil, Fail("file_limit", "File exceeds the 8 MiB read limit")
	}
	return data, r.Context.Err()
}

func fileInfo(info os.FileInfo) fileRecord {
	kind := "other"
	switch {
	case info.Mode().IsRegular():
		kind = "file"
	case info.IsDir():
		kind = "directory"
	case info.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	}
	return fileRecord{Name: info.Name(), Type: kind, Size: info.Size(), Mode: uint32(info.Mode().Perm())}
}

func (r *Runtime) statFile(root *os.Root, name string, follow bool) (*fileRecord, error) {
	if err := r.Context.Err(); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fileError(err)
	}
	if follow && info.Mode()&os.ModeSymlink != 0 {
		f, err := r.openRead(root, name)
		if ErrorCode(err) == "file_missing" {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		info, err = f.Stat()
		_ = f.Close()
		if err != nil {
			return nil, fileError(err)
		}
	}
	record := fileInfo(info)
	record.Name = filepath.Base(name)
	if record.Type == "symlink" {
		record.Target, err = root.Readlink(name)
	}
	return &record, fileError(err)
}

func (r *Runtime) listFiles(root *os.Root, name string) ([]fileRecord, error) {
	f, err := r.openRead(root, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(fileEntryLimit + 1)
	if err != nil && err != io.EOF {
		return nil, fileError(err)
	}
	if len(entries) > fileEntryLimit {
		return nil, Fail("file_limit", "Directory exceeds the 1024-entry limit")
	}
	result := make([]fileRecord, 0, len(entries))
	for _, entry := range entries {
		if err := r.Context.Err(); err != nil {
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fileError(err)
		}
		result = append(result, fileInfo(info))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *Runtime) writeFile(root *os.Root, name string, data []byte, replace bool) (fileOutcome, error) {
	result := fileOutcome{Path: filepath.ToSlash(name)}
	if len(data) > fileByteLimit {
		return result, Fail("file_limit", "Write exceeds the 8 MiB limit")
	}
	if err := mutationPath(root, name); err != nil {
		return result, err
	}
	if !replace {
		if _, err := root.Lstat(name); err == nil {
			result.Status = "preserved"
			return result, nil
		} else if !os.IsNotExist(err) {
			return result, fileError(err)
		}
	}
	if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return result, fileError(err)
	}
	id, err := ID()
	if err != nil {
		return result, err
	}
	temp := filepath.Join(filepath.Dir(name), ".wtf-write-"+id)
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return result, fileError(err)
	}
	defer func() { _ = root.Remove(temp) }()
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return result, fileError(err)
	}
	if err := r.Context.Err(); err != nil {
		return result, err
	}
	if replace {
		err = root.Rename(temp, name)
	} else {
		err = root.Link(temp, name)
	}
	if !replace && errors.Is(err, os.ErrExist) {
		result.Status = "preserved"
		return result, nil
	}
	if err != nil {
		return result, fileError(err)
	}
	result.Status = "written"
	return result, nil
}

func (r *Runtime) symlinkFile(source *os.Root, src string, dest *os.Root, dst string, replace bool) (fileOutcome, error) {
	result := fileOutcome{Path: filepath.ToSlash(dst)}
	if err := mutationPath(dest, dst); err != nil {
		return result, err
	}
	// The source may be missing, but its parent must belong to the selected root.
	parent, err := source.Open(filepath.Dir(src))
	if err != nil {
		return result, fileError(err)
	}
	_ = parent.Close()
	target := filepath.Join(source.Name(), src)
	if existing, err := dest.Readlink(dst); err == nil && existing == target {
		result.Status = "reused"
		return result, nil
	}
	if !replace {
		if _, err := dest.Lstat(dst); err == nil {
			result.Status = "preserved"
			return result, nil
		} else if !os.IsNotExist(err) {
			return result, fileError(err)
		}
	}
	if err := dest.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return result, fileError(err)
	}
	id, err := ID()
	if err != nil {
		return result, err
	}
	temp := filepath.Join(filepath.Dir(dst), ".wtf-link-"+id)
	if err := dest.Symlink(target, temp); err != nil {
		return result, fileError(err)
	}
	defer func() { _ = dest.Remove(temp) }()
	if err := r.Context.Err(); err != nil {
		return result, err
	}
	if replace {
		err = dest.Rename(temp, dst)
	} else {
		err = dest.Link(temp, dst)
	}
	if !replace && errors.Is(err, os.ErrExist) {
		result.Status = "preserved"
		return result, nil
	}
	if err != nil {
		return result, fileError(err)
	}
	result.Status = "linked"
	return result, nil
}

type copyBudget struct {
	bytes       int
	entries     int
	directories map[string]bool
}

func (r *Runtime) copyNode(source *os.Root, src string, dest *os.Root, dst string, options fileOptions, budget *copyBudget) error {
	if err := r.Context.Err(); err != nil {
		return err
	}
	budget.entries++
	if budget.entries > fileEntryLimit {
		return Fail("file_limit", "Copy exceeds 1024 entries")
	}
	info, err := source.Lstat(src)
	if err != nil {
		return fileError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		switch options.Symlinks {
		case "preserve":
			target, err := source.Readlink(src)
			if err != nil {
				return fileError(err)
			}
			return fileError(dest.Symlink(target, dst))
		case "follow":
			actual, err := filepath.EvalSymlinks(filepath.Join(source.Name(), src))
			if err != nil {
				return fileError(err)
			}
			for _, scope := range []string{"project", "workspace"} {
				allowed, err := r.fileRoot(scope)
				if err != nil {
					continue
				}
				rel, err := filepath.Rel(allowed.Name(), actual)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
					err = r.copyNode(allowed, rel, dest, dst, options, budget)
					_ = allowed.Close()
					return err
				}
				_ = allowed.Close()
			}
			return Fail("invalid_path", "Copy target escapes the allowed roots")
		default:
			return Fail("invalid_options", "Copying a symlink requires symlinks = preserve or follow")
		}
	}
	if info.IsDir() {
		if !options.Recursive {
			return Fail("invalid_options", "Directory copy requires recursive = true")
		}
		actual, err := filepath.EvalSymlinks(filepath.Join(source.Name(), src))
		if err != nil {
			return fileError(err)
		}
		if budget.directories[actual] {
			return Fail("invalid_path", "Directory copy contains a cycle")
		}
		budget.directories[actual] = true
		defer delete(budget.directories, actual)
		if err := dest.Mkdir(dst, 0755); err != nil {
			return fileError(err)
		}
		entries, err := r.listFiles(source, src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := r.copyNode(source, filepath.Join(src, entry.Name), dest, filepath.Join(dst, entry.Name), options, budget); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return Fail("invalid_file_type", "Copy supports regular files, directories and explicit symlink policies")
	}
	data, err := r.readFile(source, src)
	if err != nil {
		return err
	}
	budget.bytes += len(data)
	if budget.bytes > copyByteLimit {
		return Fail("file_limit", "Copy exceeds the 32 MiB total limit")
	}
	f, err := dest.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fileError(err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	return fileError(errors.Join(writeErr, closeErr))
}

func (r *Runtime) copyFile(source *os.Root, src string, dest *os.Root, dst string, options fileOptions) (fileOutcome, error) {
	result := fileOutcome{Path: filepath.ToSlash(dst)}
	if err := mutationPath(dest, dst); err != nil {
		return result, err
	}
	info, err := source.Lstat(src)
	if err != nil {
		return result, fileError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 && options.Symlinks == "follow" {
		f, err := r.openRead(source, src)
		if err != nil {
			return result, err
		}
		info, err = f.Stat()
		_ = f.Close()
		if err != nil {
			return result, fileError(err)
		}
	}
	if info.IsDir() {
		actual, err := filepath.EvalSymlinks(filepath.Join(source.Name(), src))
		if err != nil {
			return result, fileError(err)
		}
		rel, err := filepath.Rel(actual, filepath.Join(dest.Name(), dst))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return result, Fail("invalid_path", "Cannot copy a directory into itself")
		}
	}
	if !options.Replace {
		if _, err := dest.Lstat(dst); err == nil {
			result.Status = "preserved"
			return result, nil
		} else if !os.IsNotExist(err) {
			return result, fileError(err)
		}
	}
	if err := dest.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return result, fileError(err)
	}
	if info.IsDir() {
		actual, err := filepath.EvalSymlinks(filepath.Join(source.Name(), src))
		if err != nil {
			return result, fileError(err)
		}
		parent, err := filepath.EvalSymlinks(filepath.Join(dest.Name(), filepath.Dir(dst)))
		if err != nil {
			return result, fileError(err)
		}
		rel, err := filepath.Rel(actual, filepath.Join(parent, filepath.Base(dst)))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return result, Fail("invalid_path", "Cannot copy a directory into itself through a link")
		}
	}
	id, err := ID()
	if err != nil {
		return result, err
	}
	temp := filepath.Join(filepath.Dir(dst), ".wtf-copy-"+id)
	defer func() { _ = dest.RemoveAll(temp) }()
	if err := r.copyNode(source, src, dest, temp, options, &copyBudget{directories: map[string]bool{}}); err != nil {
		return result, err
	}
	if err := r.Context.Err(); err != nil {
		return result, err
	}
	staged, err := dest.Lstat(temp)
	if err != nil {
		return result, fileError(err)
	}
	switch {
	case options.Replace:
		err = dest.Rename(temp, dst)
	case staged.IsDir():
		err = commitNewDirectory(dest, temp, dst)
	default:
		err = dest.Link(temp, dst)
	}
	if !options.Replace && errors.Is(err, os.ErrExist) {
		result.Status = "preserved"
		return result, nil
	}
	if err != nil {
		return result, fileError(err)
	}
	result.Status = "copied"
	return result, nil
}
