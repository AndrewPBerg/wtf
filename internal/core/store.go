package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Repo struct {
	Root  string
	Store string
}

func Discover(ctx context.Context, dir string) (Repo, error) {
	root, e := output(ctx, dir, "root", "--ignore-working-copy")
	if e != nil {
		return Repo{}, Fail("not_a_jj_repository", "Run inside a JJ workspace or use --global from a parent directory")
	}
	p := filepath.Join(root, ".jj", "repo")
	st, e := os.Stat(p)
	if e != nil {
		return Repo{}, e
	}
	if !st.IsDir() {
		b, e := os.ReadFile(p)
		if e != nil {
			return Repo{}, e
		}
		p = filepath.Join(root, ".jj", strings.TrimSpace(string(b)))
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return Repo{}, e
	}
	return Repo{root, filepath.Join(p, "wtf-v1-pilot")}, nil
}
func (r Repo) Load() (State, error) {
	var s State
	e := readJSON(filepath.Join(r.Store, "state.json"), &s)
	if errors.Is(e, os.ErrNotExist) {
		return s, Fail("not_registered", "Run wtf init to opt this repository into the pilot")
	}
	if e != nil {
		return s, e
	}
	if s.Version != 1 || s.RepositoryID == "" || s.Workspaces == nil || s.Claims == nil {
		return s, Fail("invalid_registry", "Unsupported or incomplete repository registry")
	}
	return s, nil
}
func (r Repo) Edit(ctx context.Context, fn func(*State) error) error {
	if e := os.MkdirAll(r.Store, 0700); e != nil {
		return e
	}
	unlock, e := lock(ctx, filepath.Join(r.Store, "registry.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	s, e := r.Load()
	if e != nil {
		return e
	}
	if e = fn(&s); e != nil {
		return e
	}
	return writeAtomic(filepath.Join(r.Store, "state.json"), s)
}
func (r Repo) Workspace(ctx context.Context) (Workspace, error) {
	s, e := r.Load()
	if e != nil {
		return Workspace{}, e
	}
	for _, w := range s.Workspaces {
		if samePath(w.Path, r.Root) && w.Status != "removed" {
			return w, nil
		}
	}
	return Workspace{}, Fail("unmanaged_workspace", "This JJ workspace is not registered; run wtf init here")
}
func (r Repo) Update(ctx context.Context, id string, fn func(*Workspace)) error {
	return r.Edit(ctx, func(s *State) error {
		w, ok := s.Workspaces[id]
		if !ok {
			return Fail("workspace_missing", "Workspace identity is missing")
		}
		fn(&w)
		s.Workspaces[id] = w
		return nil
	})
}
func (r Repo) LockWorkspace(ctx context.Context, id string) (func(), error) {
	dir := filepath.Join(r.Store, "locks")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	return lock(ctx, filepath.Join(dir, id+".lock"))
}
func (a *App) index() (Index, error) {
	var i Index
	e := readJSON(filepath.Join(a.Home, "repositories.json"), &i)
	if errors.Is(e, os.ErrNotExist) {
		return Index{1, map[string]Repository{}}, nil
	}
	if e != nil {
		return i, e
	}
	if i.Version != 1 || i.Repositories == nil {
		return i, Fail("invalid_registry", "Unsupported global index")
	}
	return i, nil
}
func (a *App) register(ctx context.Context, r Repo, s State) error {
	if e := os.MkdirAll(a.Home, 0700); e != nil {
		return e
	}
	unlock, e := lock(ctx, filepath.Join(a.Home, "index.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	i, e := a.index()
	if e != nil {
		return e
	}
	i.Repositories[s.RepositoryID] = Repository{s.RepositoryID, s.Anchor, r.Store}
	return writeAtomic(filepath.Join(a.Home, "repositories.json"), i)
}
func NativeWorkspaces(ctx context.Context, root string) (map[string]string, error) {
	v, e := output(ctx, root, "workspace", "list", "--ignore-working-copy", "-T", `name ++ "\x1f" ++ self.root() ++ "\n"`)
	if e != nil {
		return nil, e
	}
	m := map[string]string{}
	for _, line := range strings.Split(v, "\n") {
		name, path, ok := strings.Cut(line, "\x1f")
		if !ok {
			return nil, Fail("jj_failed", "Unexpected workspace list output")
		}
		m[name] = path
	}
	return m, nil
}

type Initialization struct {
	Workspace
	Code string `json:"code"`
}

func (a *App) Initialize(ctx context.Context, args ...string) (Initialization, error) {
	code := "initialized"
	w, err := a.initialize(ctx, &code)
	if err == nil {
		var r Repo
		r, err = Discover(ctx, w.Path)
		if err == nil {
			var unlock func()
			unlock, err = r.LockWorkspace(ctx, w.ID)
			if err == nil {
				defer unlock()
				w, err = a.prepareInitialization(ctx, r, w, args...)
			}
		}
	}
	return Initialization{Workspace: w, Code: code}, err
}

func (a *App) Init(ctx context.Context) (Workspace, error) {
	result, err := a.Initialize(ctx)
	return result.Workspace, err
}

func (a *App) initialize(ctx context.Context, code *string) (Workspace, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Workspace{}, e
	}
	if e = os.MkdirAll(r.Store, 0700); e != nil {
		return Workspace{}, e
	}
	unlock, e := lock(ctx, filepath.Join(r.Store, "registry.lock"))
	if e != nil {
		return Workspace{}, e
	}
	s, e := r.Load()
	if ErrorCode(e) == "not_registered" {
		id, ie := ID()
		if ie != nil {
			unlock()
			return Workspace{}, ie
		}
		s = State{1, id, r.Root, map[string]Workspace{}, map[string]Claim{}}
	} else if e != nil {
		unlock()
		return Workspace{}, e
	}
	var w Workspace
	for _, existing := range s.Workspaces {
		if samePath(existing.Path, r.Root) && existing.Status != "removed" {
			w = existing
			*code = "already_initialized"
			break
		}
	}
	if w.ID == "" {
		native, e := NativeWorkspaces(ctx, r.Root)
		if e != nil {
			unlock()
			return w, e
		}
		name := ""
		for n, p := range native {
			if samePath(p, r.Root) {
				name = n
				break
			}
		}
		if name == "" {
			unlock()
			return w, Fail("jj_failed", "Cannot identify current native workspace")
		}
		id, e := ID()
		if e != nil {
			unlock()
			return w, e
		}
		w = Workspace{id, name, r.Root, false, "ready", map[string]Stage{}, ""}
		s.Workspaces[id] = w
	}
	e = writeAtomic(filepath.Join(r.Store, "state.json"), s)
	unlock()
	if e != nil {
		return w, e
	}
	if e = a.register(ctx, r, s); e != nil {
		return w, e
	}
	return w, nil
}

type WorkspaceView struct {
	Workspace
	RepositoryID string  `json:"repository_id"`
	Exists       bool    `json:"exists"`
	GitHub       *PRInfo `json:"github,omitempty"`
}

func views(s State) []WorkspaceView {
	out := []WorkspaceView{}
	for _, w := range s.Workspaces {
		_, e := os.Stat(w.Path)
		out = append(out, WorkspaceView{Workspace: w, RepositoryID: s.RepositoryID, Exists: e == nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (a *App) List(ctx context.Context, global, here bool) ([]WorkspaceView, error) {
	if !global {
		if r, e := Discover(ctx, a.Cwd); e == nil {
			s, e := r.Load()
			if e != nil {
				return nil, e
			}
			if here {
				w, e := r.Workspace(ctx)
				if e != nil {
					return nil, e
				}
				s.Workspaces = map[string]Workspace{w.ID: w}
			}
			return views(s), nil
		}
	}
	i, e := a.index()
	if e != nil {
		return nil, e
	}
	out := []WorkspaceView{}
	for _, entry := range i.Repositories {
		if !global {
			rel, e := filepath.Rel(a.Cwd, entry.Path)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				continue
			}
		}
		s, e := (Repo{entry.Path, entry.Store}).Load()
		if e != nil {
			return nil, fmt.Errorf("%s: %w", entry.Path, e)
		}
		out = append(out, views(s)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
func (r Repo) Select(name string) (Workspace, error) {
	s, e := r.Load()
	if e != nil {
		return Workspace{}, e
	}
	for _, w := range s.Workspaces {
		if w.Status != "removed" && (w.Name == name || w.ID == name) {
			return w, nil
		}
	}
	return Workspace{}, Fail("workspace_missing", "No managed workspace matches "+name)
}
func (r Repo) Claim(ctx context.Context, w Workspace, kind, name, value string) (Claim, error) {
	if e := CheckName(kind); e != nil {
		return Claim{}, e
	}
	if e := CheckName(name); e != nil {
		return Claim{}, e
	}
	if value == "" {
		return Claim{}, Fail("invalid_claim", "Claim value is required")
	}
	c := Claim{w.ID, kind, name, value}
	e := r.Edit(ctx, func(s *State) error {
		current, ok := s.Workspaces[w.ID]
		if !ok || current.Status == "removed" {
			return Fail("workspace_missing", "Cannot claim resources for a removed workspace")
		}
		key := w.ID + ":" + kind + ":" + name
		if old, ok := s.Claims[key]; ok && old != c {
			return Fail("claim_conflict", "Release the previous named claim before replacing it")
		}
		for _, old := range s.Claims {
			if old.Kind == kind && old.Value == value && old.Owner != w.ID {
				return Fail("claim_conflict", "Resource is already claimed by another workspace")
			}
		}
		s.Claims[key] = c
		return nil
	})
	return c, e
}
func (r Repo) Release(ctx context.Context, w Workspace, kind, name string) error {
	return r.Edit(ctx, func(s *State) error { delete(s.Claims, w.ID+":"+kind+":"+name); return nil })
}
func (r Repo) Claims(w Workspace) ([]Claim, error) {
	s, e := r.Load()
	if e != nil {
		return nil, e
	}
	out := []Claim{}
	for _, c := range s.Claims {
		if c.Owner == w.ID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+out[i].Name < out[j].Kind+out[j].Name })
	return out, nil
}
