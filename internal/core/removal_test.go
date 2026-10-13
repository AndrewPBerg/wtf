package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func removalCandidateByID(t *testing.T, a *App, id string) RemovalCandidate {
	t.Helper()
	rows, err := a.RemovalCandidates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatal("candidate was not returned")
	return RemovalCandidate{}
}

func TestRemovalReusesRecentMetadataAcrossWorkspaceInspections(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../first", "../second"} {
		if _, err := a.NewWorkspace(t.Context(), name, "@"); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		calls++
		return []PullRequest{}, nil
	}}
	rows, err := a.List(t.Context(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := 0
	for _, row := range rows {
		candidate, err := a.InspectRemoval(t.Context(), row)
		if err != nil || candidate.ID != row.ID {
			t.Fatalf("single workspace inspection: %+v %v", candidate, err)
		}
		if row.Owned && row.Exists && !samePath(row.Path, r.Root) {
			wantCalls = 1
		}
		if calls != wantCalls {
			t.Fatalf("metadata requests: got %d, want %d", calls, wantCalls)
		}
	}
	if wantCalls != 1 {
		t.Fatalf("expected one shared metadata request, got %d", wantCalls)
	}
}

func TestRemovalReusesBrowserObservationThroughDeletion(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(t.Context(), "../published", "@")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("published"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), child.Path, "status"); err != nil {
		t.Fatal(err)
	}
	revision, err := removalRevision(t.Context(), child.Path)
	if err != nil {
		t.Fatal(err)
	}
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		return []PullRequest{{Number: 42, Head: revision.Commit, State: "MERGED", Base: "main"}}, nil
	}}
	rows, err := a.List(t.Context(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	a.Metadata().Observe(t.Context(), rows, MetadataRequest{PRs: true, CI: true})
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		t.Error("removal refetched the browser's GitHub observation")
		return nil, Fail("unexpected_request", "no provider requests allowed")
	}}
	candidate := removalCandidateByID(t, a, child.ID)
	if !candidate.Integrated || candidate.GitHub.Lookup != "cached" || candidate.Evidence != "merged_pr:42" {
		t.Fatalf("cached proof not reused: %+v", candidate)
	}
	if _, err = a.RemoveWithOptions(t.Context(), child.ID, candidate.RemovalOptions(false)); err != nil {
		t.Fatal(err)
	}
}

func TestRemovalPreflightCannotHideLaterChangesOrAnotherWorkspace(t *testing.T) {
	for _, mode := range []string{"disk-edit", "hook-edit", "other-workspace"} {
		t.Run(mode, func(t *testing.T) {
			a, r, _ := fixture(t)
			if mode == "hook-edit" {
				config(t, r.Root, `return { overrides = { remove = function(ctx, request, default)
ctx:stage("source-change", {"sh", "-c", "printf changed > input.txt"})
return default(request)
end } }`)
			}
			child, err := a.NewWorkspace(t.Context(), "../checked", "@")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = output(t.Context(), child.Path, "status"); err != nil {
				t.Fatal(err)
			}
			revision, err := removalRevision(t.Context(), child.Path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = output(t.Context(), r.Root, "bookmark", "create", "main", "-r", revision.Commit); err != nil {
				t.Fatal(err)
			}
			candidate := removalCandidateByID(t, a, child.ID)
			if !candidate.Integrated {
				t.Fatalf("preflight failed: %+v", candidate)
			}
			target := child.Workspace
			want := "workspace_has_changes"
			if mode == "disk-edit" {
				if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("new edit"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "other-workspace" {
				other, err := a.NewWorkspace(t.Context(), "../other", "@")
				if err != nil {
					t.Fatal(err)
				}
				target, want = other.Workspace, "inspection_failed"
			}
			a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
				t.Error("deletion fetched GitHub")
				return nil, nil
			}}
			if _, err = a.RemoveWithOptions(t.Context(), target.ID, candidate.RemovalOptions(false)); ErrorCode(err) != want {
				t.Fatalf("got %v, want %s", err, want)
			}
			if _, err = os.Stat(target.Path); err != nil {
				t.Fatal("workspace was removed", err)
			}
		})
	}
}

func TestRemovalForceMakesNoProviderRequests(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		t.Error("forced deletion fetched GitHub")
		return nil, nil
	}}
	var children []Workspace
	for i := 0; i < 7; i++ {
		child, err := a.NewWorkspace(t.Context(), "../discard-"+string(rune('a'+i)), "@")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("discard"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = output(t.Context(), child.Path, "status"); err != nil {
			t.Fatal(err)
		}
		children = append(children, child.Workspace)
	}
	start := time.Now()
	for _, child := range children {
		if _, err := a.RemoveWithOptions(t.Context(), child.ID, RemovalOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("seven forced removals, zero provider requests: %s", time.Since(start))
}

func TestRemovalRejectsStaleAndFailedProviderProof(t *testing.T) {
	now, stale := time.Now(), time.Now().Add(-3*time.Minute)
	for _, info := range []*PRInfo{
		{Lookup: "cached", CheckedAt: &now}, {Lookup: "fresh", CheckedAt: &now},
	} {
		if !recentRemovalMetadata(info) {
			t.Fatal("recent successful observation rejected")
		}
	}
	for _, info := range []*PRInfo{nil, {Lookup: "cached"}, {Lookup: "cached", CheckedAt: &stale}, {Lookup: "stale", CheckedAt: &now}, {Lookup: "fresh", CheckedAt: &now, Error: "offline"}} {
		if recentRemovalMetadata(info) {
			t.Fatal("invalid observation accepted")
		}
	}
}

func TestRemovalObservesMainAndBlocksNewChanges(t *testing.T) {
	a, r, w := fixture(t)
	if _, err := output(t.Context(), r.Root, "bookmark", "create", "main", "-r", "@"); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(t.Context(), "../finished", "@")
	if err != nil {
		t.Fatal(err)
	}
	row := removalCandidateByID(t, a, child.ID)
	if !row.Integrated || !row.Selectable || row.Evidence != "jj_ancestry" {
		t.Fatalf("clean integrated workspace: %+v", row)
	}
	if current := removalCandidateByID(t, a, w.ID); current.Selectable || current.Code != "current_workspace" {
		t.Fatalf("current workspace selectable: %+v", current)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("unpublished"), 0600); err != nil {
		t.Fatal(err)
	}
	row = removalCandidateByID(t, a, child.ID)
	if row.Selectable || row.Integrated || len(row.Changes) == 0 || !row.ObservedEdits {
		t.Fatalf("new changes treated as safe: %+v", row)
	}
	if _, err = a.Remove(t.Context(), child.ID); ErrorCode(err) != "workspace_has_changes" {
		t.Fatalf("dirty removal: %v", err)
	}
	if _, err = os.Stat(child.Path); err != nil {
		t.Fatal("dirty workspace was removed", err)
	}
}

func TestRemovalRecognizesExactMergedPRAndRejectsLaterEdits(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	child, err := a.NewWorkspace(t.Context(), "../published", "@")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("published"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), child.Path, "status"); err != nil {
		t.Fatal(err)
	}
	published, err := removalRevision(t.Context(), child.Path)
	if err != nil {
		t.Fatal(err)
	}
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		return []PullRequest{{Number: 42, Head: published.Commit, State: "MERGED", Base: "main"}}, nil
	}}
	row := removalCandidateByID(t, a, child.ID)
	if !row.Integrated || !row.Selectable || row.Evidence != "merged_pr:42" {
		t.Fatalf("merged exact head not recognized: %+v", row)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("post-merge changes"), 0600); err != nil {
		t.Fatal(err)
	}
	row = removalCandidateByID(t, a, child.ID)
	if row.Integrated || row.Selectable || !row.ObservedEdits {
		t.Fatalf("merged PR hid newer edits: %+v", row)
	}
	if _, err = a.Remove(t.Context(), child.ID); ErrorCode(err) != "workspace_has_changes" {
		t.Fatalf("later edits removed: %v", err)
	}
}

func TestRemovalCanDeleteAnIntegratedNonemptyRevision(t *testing.T) {
	a, r, _ := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "../merged", "@")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("merged"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), child.Path, "status"); err != nil {
		t.Fatal(err)
	}
	revision, err := removalRevision(t.Context(), child.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), r.Root, "bookmark", "create", "main", "-r", revision.Commit); err != nil {
		t.Fatal(err)
	}
	if removed, err := a.Remove(t.Context(), child.ID); err != nil || removed.Status != "removed" {
		t.Fatalf("integrated revision cleanup: %+v %v", removed, err)
	}
}

func TestRemovalRechecksChangesImmediatelyBeforeDeletion(t *testing.T) {
	a, r, _ := fixture(t)
	config(t, r.Root, `return { overrides = { remove = function(ctx, request, default)
ctx:stage("source-change", {"sh", "-c", "printf changed > input.txt"})
return default(request)
end } }`)
	child, err := a.NewWorkspace(t.Context(), "../integrated", "@")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("merged"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), child.Path, "status"); err != nil {
		t.Fatal(err)
	}
	revision, err := removalRevision(t.Context(), child.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = output(t.Context(), r.Root, "bookmark", "create", "main", "-r", revision.Commit); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Remove(t.Context(), child.ID); ErrorCode(err) != "workspace_has_changes" {
		t.Fatalf("source changes during removal were not blocked: %v", err)
	}
	if _, err = os.Stat(child.Path); err != nil {
		t.Fatal("changed workspace was deleted", err)
	}
}

func TestRemovalForceDiscardsChangesButKeepsProtectedGuards(t *testing.T) {
	a, r, current := fixture(t)
	child, err := a.NewWorkspace(t.Context(), "../discard", "@")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(child.Path, "input.txt"), []byte("discard me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Remove(t.Context(), child.ID); ErrorCode(err) != "workspace_has_changes" {
		t.Fatalf("normal removal must reject changes: %v", err)
	}
	if _, err = a.RemoveWithOptions(t.Context(), current.ID, RemovalOptions{Force: true}); err == nil {
		t.Fatal("force removed protected/current workspace")
	}
	if _, err = os.Stat(r.Root); err != nil {
		t.Fatal("protected directory lost", err)
	}
	removed, err := a.RemoveWithOptions(t.Context(), child.ID, RemovalOptions{Force: true})
	if err != nil || removed.Status != "removed" {
		t.Fatalf("forced removal: %+v %v", removed, err)
	}
	if _, err = os.Stat(child.Path); !os.IsNotExist(err) {
		t.Fatalf("forced workspace directory remains: %v", err)
	}
}

func TestRemovalForceCannotBypassOtherAssessmentFailures(t *testing.T) {
	for _, code := range []string{"checking", "protected_workspace", "current_workspace", "identity_mismatch", "workspace_unavailable", "inspection_failed"} {
		if (RemovalCandidate{Code: code}).CanRemove(true) {
			t.Fatalf("force bypassed %s", code)
		}
	}
}
