package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type fakePRClient struct {
	call func(context.Context, string) ([]PullRequest, error)
}

func (f fakePRClient) List(ctx context.Context, repo string) ([]PullRequest, error) {
	return f.call(ctx, repo)
}
func pr(number int, branch, head, state string) PullRequest {
	return PullRequest{Number: number, Branch: branch, Head: head, State: state}
}
func TestPRAssociationUsesBookmarksOrExactRevisionsNotWorkspaceNames(t *testing.T) {
	snapshot := prSnapshot{Name: "feature", Commit: "local", Empty: true, Parents: []prRevision{{Commit: "published", Bookmarks: []string{"topic"}}}}
	draft := pr(12, "topic", "published", "OPEN")
	draft.Draft = true
	wrong := pr(13, "feature", "unrelated", "OPEN")
	fork := pr(14, "topic", "fork-head", "OPEN")
	fork.CrossRepository = true
	fork.Owner.Login = "someone-else"
	matches := associate(snapshot, []PullRequest{draft, wrong, fork}, "owner/repo")
	if len(matches) != 1 || matches[0].Number != 12 || matches[0].Relation != "base_matches" {
		t.Fatalf("bad association %+v", matches)
	}
	snapshot.Empty = false
	matches = associate(snapshot, []PullRequest{draft}, "owner/repo")
	if matches[0].Relation != "local_differs" {
		t.Fatal(matches)
	}
	snapshot.Commit = "published"
	matches = associate(snapshot, []PullRequest{draft}, "owner/repo")
	if matches[0].Relation != "head_matches" {
		t.Fatal(matches)
	}
}
func TestPRRefreshIsConcurrentBoundedAndCachesResults(t *testing.T) {
	var active, peak, calls atomic.Int32
	client := fakePRClient{func(ctx context.Context, repo string) ([]PullRequest, error) {
		n := active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-time.After(15 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []PullRequest{pr(1, "topic", "published", "OPEN")}, nil
	}}
	now := time.Now()
	service := PRService{t.TempDir(), client, func() time.Time { return now }, io.Discard}
	plan := PRPlan{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		plan.Groups = append(plan.Groups, prGroup{ID: id, Repository: "owner/" + id, Refresh: true, Snapshot: map[string]prSnapshot{"work": {Name: "work", Commit: "published"}}})
		plan.Rows = append(plan.Rows, WorkspaceView{Workspace: Workspace{Name: "work", Status: "ready"}, RepositoryID: id, Exists: true})
	}
	rows := service.Refresh(context.Background(), plan)
	if peak.Load() < 2 || peak.Load() > prWorkers || calls.Load() != 6 {
		t.Fatalf("peak=%d calls=%d", peak.Load(), calls.Load())
	}
	if len(service.cache().Repositories) != 6 {
		t.Fatal("cache lost concurrent results")
	}
	for _, row := range rows {
		if row.GitHub.Lookup != "fresh" || len(row.GitHub.Matches) != 1 {
			t.Fatalf("bad result %+v", row.GitHub)
		}
	}
}
func TestPRFailureRetainsStaleCacheAndDoesNotInventNoPR(t *testing.T) {
	now := time.Now()
	old := prCacheEntry{"owner/repo", now.Add(-time.Hour), []PullRequest{pr(4, "topic", "published", "OPEN")}}
	service := PRService{t.TempDir(), fakePRClient{func(ctx context.Context, repo string) ([]PullRequest, error) { return nil, Fail("offline", "offline") }}, func() time.Time { return now }, io.Discard}
	plan := PRPlan{Groups: []prGroup{{ID: "r", Repository: "owner/repo", Cached: &old, Refresh: true, Snapshot: map[string]prSnapshot{"work": {Commit: "published"}}}}, Rows: []WorkspaceView{{Workspace: Workspace{Name: "work", Status: "ready"}, RepositoryID: "r", Exists: true}}}
	row := service.Refresh(context.Background(), plan)[0]
	if row.GitHub.Lookup != "stale" || row.GitHub.Error == "" || len(row.GitHub.Matches) != 1 {
		t.Fatalf("stale fallback %+v", row.GitHub)
	}
	plan.Groups[0].Cached = nil
	row = service.Refresh(context.Background(), plan)[0]
	if row.GitHub.Lookup != "unavailable" {
		t.Fatalf("error became no PR %+v", row.GitHub)
	}
}
func TestPRPrepareSkipsNetworkForFreshCache(t *testing.T) {
	a, r, w := fixture(t)
	if _, e := output(context.Background(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); e != nil {
		t.Fatal(e)
	}
	state, e := r.Load()
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	var calls atomic.Int32
	service := PRService{a.Home, fakePRClient{func(context.Context, string) ([]PullRequest, error) { calls.Add(1); return nil, nil }}, func() time.Time { return now }, io.Discard}
	if e = service.save(context.Background(), map[string]prCacheEntry{state.RepositoryID: {"owner/repo", now, []PullRequest{}}}); e != nil {
		t.Fatal(e)
	}
	rows := []WorkspaceView{{Workspace: w, RepositoryID: state.RepositoryID, Exists: true}}
	plan := service.Prepare(context.Background(), rows, false)
	if plan.NeedsRefresh || plan.Rows[0].GitHub.Lookup != "cached" || calls.Load() != 0 {
		t.Fatalf("cache not lazy %+v calls=%d", plan, calls.Load())
	}
	if !service.Prepare(context.Background(), rows, true).NeedsRefresh {
		t.Fatal("force refresh ignored")
	}
}
func TestGitHubURLParsing(t *testing.T) {
	for _, remote := range []string{"https://github.com/owner/repo.git", "git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git"} {
		if githubRepository(remote) != "owner/repo" {
			t.Fatal(remote)
		}
	}
	for _, remote := range []string{"https://gitlab.com/owner/repo", "https://github.com.evil.test/owner/repo", "https://github.com/owner/repo/extra"} {
		if githubRepository(remote) != "" {
			t.Fatal("bad remote accepted", remote)
		}
	}
}

func TestGHClientResolvesInstalledMiseBinaryWithoutSetupLauncher(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual-gh")
	marker := filepath.Join(dir, "setup-was-run")
	scripts := map[string]string{
		"mise":      "#!/bin/sh\nprintf '%s\\n' \"$WTF_TEST_GH_BINARY\"\n",
		"gh":        "#!/bin/sh\ntouch \"$WTF_TEST_SETUP_MARKER\"\nprintf 'setup banner\\n[]\\n'\n",
		"actual-gh": "#!/bin/sh\nprintf '%s\\n' '[{\"number\":17,\"headRefName\":\"topic\",\"headRefOid\":\"published\",\"state\":\"OPEN\",\"isDraft\":true}]'\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WTF_TEST_GH_BINARY", actual)
	t.Setenv("WTF_TEST_SETUP_MARKER", marker)
	prs, err := (GHClient{}).List(context.Background(), "owner/repo")
	if err != nil || len(prs) != 1 || prs[0].Number != 17 {
		t.Fatalf("direct binary failed: %+v %v", prs, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("metadata read executed setup launcher")
	}
}
