package core

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type fakeReviewClient struct {
	resolve  func(context.Context, string, string) ReviewResolution
	diagnose func(context.Context, string) ReviewResolution
}

func (c fakeReviewClient) Resolve(ctx context.Context, root, uri string) ReviewResolution {
	return c.resolve(ctx, root, uri)
}
func (c fakeReviewClient) Diagnose(ctx context.Context, root string) ReviewResolution {
	return c.diagnose(ctx, root)
}

func TestReviewLinksPreferLinearPreserveChecksAndCachePerRepository(t *testing.T) {
	var calls atomic.Int32
	now := time.Now().UTC()
	a := New(t.TempDir(), t.TempDir(), nil)
	a.Reviews = fakeReviewClient{resolve: func(_ context.Context, root, uri string) ReviewResolution {
		calls.Add(1)
		return ReviewResolution{Provider: "linear", URL: "https://linear.app/cascadinglabs/review/example", Code: "review_found", Workspace: "cascadinglabs", WorkspaceID: "cl-id", Server: "linear", CheckedAt: now}
	}}
	rows := func(root string) []WorkspaceView {
		return []WorkspaceView{{Workspace: Workspace{Path: root}, GitHub: &PRInfo{Matches: []PullRequest{{Number: 19, URL: "https://github.com/AndrewPBerg/wtf/pull/19", State: "OPEN", Checks: &CheckSummary{Failed: 1}}}}}}
	}
	first := a.WithReviewLinks(t.Context(), rows(a.Cwd), false)
	for _, row := range first {
		for _, pr := range row.GitHub.Matches {
			if pr.Review == nil || pr.Review.Provider != "linear" || pr.URL != "https://github.com/AndrewPBerg/wtf/pull/19" || pr.State != "OPEN" || pr.Checks.Failed != 1 {
				t.Fatalf("review mapping replaced PR data: %+v", pr)
			}
		}
	}
	a.WithReviewLinks(t.Context(), rows(a.Cwd), false)
	if calls.Load() != 1 {
		t.Fatal("fresh review cache was not reused")
	}
	a.WithReviewLinks(t.Context(), rows(t.TempDir()), false)
	if calls.Load() != 2 {
		t.Fatal("different repositories shared a review cache entry")
	}
	a.WithReviewLinks(t.Context(), rows(a.Cwd), true)
	if calls.Load() != 3 {
		t.Fatal("explicit refresh did not recheck the review")
	}
}

func TestReviewFallbackDoesNotTrustWrongWorkspaceOrUnsafeURL(t *testing.T) {
	for _, result := range []ReviewResolution{
		{Provider: "github", Code: "review_not_found"},
		{Provider: "linear", URL: "https://linear.app/alitahealth/review/example", Workspace: "alitahealth", WorkspaceID: "wrong"},
		{Provider: "linear", URL: "https://linear.app.attacker.test/cascadinglabs/review/example", Workspace: "cascadinglabs", WorkspaceID: "cl-id"},
	} {
		a := New(t.TempDir(), t.TempDir(), nil)
		a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution { return result }}
		rows := a.WithReviewLinks(t.Context(), []WorkspaceView{{Workspace: Workspace{Path: a.Cwd}, GitHub: &PRInfo{Matches: []PullRequest{{URL: "https://github.com/owner/repo/pull/1"}}}}}, false)
		for _, row := range rows {
			for _, pr := range row.GitHub.Matches {
				if pr.Review == nil || pr.Review.Provider != "github" || pr.Review.URL != pr.URL {
					t.Fatalf("unsafe review destination accepted: %+v", pr.Review)
				}
			}
		}
	}
}

func TestLinearDoctorWarningsRemainOptionalAndExposeIdentity(t *testing.T) {
	for _, status := range []string{"ready", "unavailable"} {
		a := New(t.TempDir(), t.TempDir(), nil)
		a.Reviews = fakeReviewClient{diagnose: func(context.Context, string) ReviewResolution {
			return ReviewResolution{Status: status, Code: "workspace_mismatch", Workspace: "cascadinglabs", WorkspaceID: "cl-id", Server: "linear"}
		}}
		check := a.linearToolCheck(t.Context())
		if check.Required || check.Status != status || check.Review == nil || check.Review.WorkspaceID != "cl-id" {
			t.Fatalf("review diagnostics lost scope or became required: %+v", check)
		}
	}
}

func TestEmbeddedBridgeReportsMissingRuntimeWithoutTouchingRepository(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	result := (NativeReviewClient{}).Resolve(t.Context(), root, "https://github.com/owner/repo/pull/1")
	if result.Code != "node_missing" || result.Provider != "github" {
		t.Fatalf("missing runtime did not fall back: %+v", result)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatalf("resolver changed the repository: %v", err)
	}
	if validLinearReview("https://linear.app/cascadinglabs/review/x\x1b]8;;evil", "cascadinglabs") {
		t.Fatal("terminal control injection was accepted")
	}
}

func reviewTestRows(root string) []WorkspaceView {
	return []WorkspaceView{{Workspace: Workspace{Path: root}, GitHub: &PRInfo{Matches: []PullRequest{{URL: "https://github.com/owner/repo/pull/1"}}}}}
}

func TestReviewRefreshPreservesPreviousBrowserSnapshot(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution {
		return ReviewResolution{Provider: "github", Code: "review_not_found", CheckedAt: time.Now().UTC()}
	}}
	previous := reviewTestRows(a.Cwd)
	previous[0].GitHub.Matches[0].Review = &ReviewResolution{Provider: "github", Code: "old_observation"}
	updated := a.WithReviewLinks(t.Context(), previous, true)
	if previous[0].GitHub.Matches[0].Review.Code != "old_observation" {
		t.Fatal("refresh changed the browser's previous observation")
	}
	if updated[0].GitHub.Matches[0].Review.Code != "review_not_found" {
		t.Fatal("refresh did not attach the new observation")
	}
}

func TestReviewTransientFailureRetriesAndInvalidatesPreviousLink(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	calls := 0
	a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution {
		calls++
		if calls == 1 || calls == 3 {
			return ReviewResolution{Provider: "github", Code: "linear_lookup_timeout", CheckedAt: time.Now().UTC()}
		}
		return ReviewResolution{Provider: "linear", URL: "https://linear.app/cascadinglabs/review/example", Code: "review_found", Workspace: "cascadinglabs", WorkspaceID: "cl-id", CheckedAt: time.Now().UTC()}
	}}
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	resolved := a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	if calls != 2 || resolved[0].GitHub.Matches[0].Review.Provider != "linear" {
		t.Fatal("transient failure was cached as a missing review")
	}
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), true)
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	if calls != 4 {
		t.Fatal("failed forced refresh left the previous link cached")
	}
}

func TestReviewCanceledLookupDoesNotPersistFallback(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution {
		cancel()
		return ReviewResolution{Provider: "github", Code: "review_not_found", CheckedAt: time.Now().UTC()}
	}}
	a.WithReviewLinks(ctx, reviewTestRows(a.Cwd), false)
	if _, err := os.Stat(filepath.Join(a.Home, "review-links.json")); !os.IsNotExist(err) {
		t.Fatalf("canceled lookup persisted a cache: %v", err)
	}
	result := (NativeReviewClient{}).Resolve(ctx, a.Cwd, "https://github.com/owner/repo/pull/1")
	if result.Code != "review_lookup_cancelled" || result.Provider != "github" {
		t.Fatalf("cancellation did not stop the adapter: %+v", result)
	}
}

func TestReviewFutureCacheTimestampForcesLookup(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	calls := 0
	a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution {
		calls++
		checkedAt := time.Now().UTC()
		if calls == 1 {
			checkedAt = checkedAt.Add(time.Hour)
		}
		return ReviewResolution{Provider: "github", Code: "review_not_found", CheckedAt: checkedAt}
	}}
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	if calls != 2 {
		t.Fatal("a future timestamp incorrectly kept the review cache fresh")
	}
	a.WithReviewLinks(t.Context(), reviewTestRows(a.Cwd), false)
	if calls != 2 {
		t.Fatal("refresh did not replace the future timestamp with a valid observation")
	}
}
