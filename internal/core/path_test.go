package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewUsesExactDestinationPathsAndPreservesHookRequests(t *testing.T) {
	for _, destination := range []string{"./LocalCase", "../SiblingCase", "absolute"} {
		t.Run(destination, func(t *testing.T) {
			a, r, anchor := fixture(t)
			path := destination
			want := filepath.Join(a.Cwd, path)
			if destination == "absolute" {
				path = filepath.Join(filepath.Dir(a.Cwd), "AbsoluteCase")
				want = path
			}
			config(t, anchor.Path, `return {overrides={
 new=function(ctx,request,default)
  ctx:require(request.path == ctx.project.path .. "/LocalCase" or request.path:match("/SiblingCase$") ~= nil or request.path:match("/AbsoluteCase$") ~= nil, "Wrong destination")
  ctx:require(request.from == "@" and request.args[1] == "--KeepCase", "Request changed")
  default(request)
 end,
 init=function(ctx,request,default)
  default(request)
  ctx:stage("prepare", {"sh","-c","printf '%s' \"$1\" > args.count","sh",request.args[1] or ""})
 end}}`)
			child, err := a.NewWorkspace(t.Context(), path, "@", "--KeepCase")
			if err != nil || child.Path != want || child.Name != filepath.Base(want) || !child.Owned || child.Code != "initialized" {
				t.Fatalf("destination was not used exactly: %+v %v; want %s", child, err, want)
			}
			args, err := os.ReadFile(filepath.Join(want, "args.count"))
			if err != nil || string(args) != "--KeepCase" {
				t.Fatalf("initialization did not run in target: %q %v", args, err)
			}
			native, err := NativeWorkspaces(t.Context(), anchor.Path)
			if err != nil || native[child.Name] != want {
				t.Fatalf("native workspace path differs: %+v %v", native, err)
			}
			if _, err := r.Select(child.Name); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewBareNamesStayBesideRepositoryFromEveryWorkspace(t *testing.T) {
	a, r, anchor := fixture(t)
	parent := filepath.Dir(anchor.Path)
	first, err := a.NewWorkspace(t.Context(), "FirstCase", "@")
	if err != nil || first.Path != filepath.Join(parent, "FirstCase") {
		t.Fatalf("first workspace is not a sibling: %+v %v", first, err)
	}
	// Even an explicitly placed workspace must not become the placement anchor.
	if err := os.Mkdir(filepath.Join(anchor.Path, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	nested, err := a.NewWorkspace(t.Context(), "./nested/ExplicitCase", "@")
	if err != nil {
		t.Fatal(err)
	}
	for i, cwd := range []string{anchor.Path, first.Path, nested.Path} {
		sub := filepath.Join(cwd, "subdirectory")
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatal(err)
		}
		a.Cwd = sub
		name := []string{"FromRepo", "FromSibling", "FromNested"}[i]
		child, err := a.NewWorkspace(t.Context(), name, "@", "--KeepCase")
		want := filepath.Join(parent, name)
		if err != nil || child.Path != want || child.Name != name || !child.Owned {
			t.Fatalf("creation from %s: %+v %v; want %s", sub, child, err, want)
		}
		repeated, err := a.NewWorkspace(t.Context(), name, "")
		if err != nil || repeated.Code != "already_initialized" || repeated.ID != child.ID {
			t.Fatalf("bare name retry changed identity: %+v %v", repeated, err)
		}
		native, err := NativeWorkspaces(t.Context(), anchor.Path)
		if err != nil || native[name] != want {
			t.Fatalf("native placement differs: %+v %v", native, err)
		}
		if _, err := r.Select(name); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(sub, name)); !os.IsNotExist(err) {
			t.Fatalf("destination was also created inside invoking directory: %v", err)
		}
	}
}

func TestNewRegistersExistingNativeWorkspaceWithoutTakingOwnership(t *testing.T) {
	a, r, anchor := fixture(t)
	path := filepath.Join(filepath.Dir(anchor.Path), "ExistingCase")
	if _, err := output(t.Context(), anchor.Path, "workspace", "add", "--name", "NativeName", "-r", "@", path); err != nil {
		t.Fatal(err)
	}
	config(t, path, `return {overrides={new=function(ctx) ctx:block("Must not create") end, init=function(ctx,request,default)
 default(request)
 ctx:stage("prepare", {"sh","-c","touch prepared.count"})
end}}`)
	// Existing paths also work from outside any repository.
	a.Cwd = filepath.Dir(anchor.Path)
	first, err := a.NewWorkspace(t.Context(), "ExistingCase", "")
	if err != nil || first.Code != "initialized" || first.Name != "NativeName" || first.Owned || first.Stages["prepare"].Status != "succeeded" {
		t.Fatalf("existing workspace not prepared correctly: %+v %v", first, err)
	}
	link := filepath.Join(a.Cwd, "alias")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	again, err := a.NewWorkspace(t.Context(), link, "")
	if err != nil || again.Code != "already_initialized" || again.ID != first.ID || again.Owned || again.Path != path {
		t.Fatalf("symlink retry changed identity: %+v %v", again, err)
	}
	state, err := r.Load()
	if err != nil || len(state.Workspaces) != 2 {
		t.Fatalf("duplicate registrations: %+v %v", state, err)
	}
	a.Cwd = anchor.Path
	if _, err := a.Remove(t.Context(), first.Name); ErrorCode(err) != "protected_workspace" {
		t.Fatalf("registration enabled deletion: %v", err)
	}
}

func TestNewRejectsOccupiedPathsAndSubdirectoriesWithoutMutation(t *testing.T) {
	a, r, anchor := fixture(t)
	subdirectory := filepath.Join(anchor.Path, "subdirectory")
	if err := os.Mkdir(subdirectory, 0700); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(filepath.Dir(anchor.Path), "plain")
	if err := os.Mkdir(plain, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(plain, "keep")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(anchor.Path), "dangling")
	if err := os.Symlink(filepath.Join(plain, "missing"), link); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ path, code string }{
		{plain, "not_a_jj_repository"}, {file, "path_exists"}, {link, "path_exists"},
		{subdirectory, "not_workspace_root"}, {"", "invalid_path"},
	} {
		if _, err := a.NewWorkspace(t.Context(), check.path, "@"); ErrorCode(err) != check.code {
			t.Fatalf("%s: want %s, got %v", check.path, check.code, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("invalid destination changed registry: %v", err)
	}
	contents, err := os.ReadFile(file)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("occupied path changed: %q %v", contents, err)
	}
}

func TestNewRejectsDuplicateNativeBasenamesBeforeRegistration(t *testing.T) {
	a, r, anchor := fixture(t)
	path := filepath.Join(filepath.Dir(anchor.Path), "native")
	if _, err := output(t.Context(), anchor.Path, "workspace", "add", "--name", "DuplicateCase", "-r", "@", path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWorkspace(t.Context(), "../DuplicateCase", "@"); ErrorCode(err) != "workspace_exists" {
		t.Fatalf("duplicate native name: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("duplicate name changed registry: %v", err)
	}
}

func TestNewOverrideCannotRedirectDestination(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `return {overrides={new=function(ctx,request,default) request.path="Elsewhere"; default(request) end}}`)
	if _, err := a.NewWorkspace(t.Context(), "../TargetCase", "@"); ErrorCode(err) != "identity_mismatch" {
		t.Fatalf("override redirected destination: %v", err)
	}
	state, err := r.Load()
	if err != nil || len(state.Workspaces) != 1 {
		t.Fatalf("redirect mutated registry: %+v %v", state, err)
	}
}
