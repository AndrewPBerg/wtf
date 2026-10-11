package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataBundlesPRsCIAndReviewsByRepository(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewWorkspace(t.Context(), "../feature", "@"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := snapshots(t.Context(), r.Root)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.List(t.Context(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	var githubCalls, reviewCalls atomic.Int32
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		githubCalls.Add(1)
		return []PullRequest{{Number: 7, URL: "https://github.com/owner/repo/pull/7", Head: snapshots["default"].Commit, State: "OPEN", Checks: &CheckSummary{Passed: 2}}}, nil
	}}
	a.Reviews = fakeReviewClient{resolve: func(context.Context, string, string) ReviewResolution {
		reviewCalls.Add(1)
		return ReviewResolution{Provider: "github", Code: "review_not_found", CheckedAt: time.Now().UTC()}
	}}
	plan := a.Metadata().Prepare(t.Context(), rows, MetadataRequest{PRs: true, CI: true, Reviews: true, ForceReviews: true})
	if githubCalls.Load() != 0 || reviewCalls.Load() != 0 {
		t.Fatal("preparing a preview made provider requests")
	}
	fresh := plan.Refresh(t.Context())
	if githubCalls.Load() != 1 || reviewCalls.Load() != 1 {
		t.Fatalf("shared requests: GitHub=%d reviews=%d", githubCalls.Load(), reviewCalls.Load())
	}
	for _, row := range fresh {
		if row.GitHub.Lookup != "fresh" || len(row.GitHub.Matches) != 1 || row.GitHub.Matches[0].Checks.Passed != 2 || row.GitHub.Matches[0].Review == nil {
			t.Fatalf("incomplete metadata: %+v", row.GitHub)
		}
	}
	for _, row := range plan.Cached {
		if row.GitHub.Lookup == "fresh" || len(row.GitHub.Matches) != 0 {
			t.Fatal("live refresh changed the rendered preview")
		}
	}
	// CI alone inherits PR association, without touching the review provider.
	ci := a.Metadata().Observe(t.Context(), rows, MetadataRequest{CI: true})
	if githubCalls.Load() != 2 || reviewCalls.Load() != 1 {
		t.Fatal("CI-only observation did not refresh once or fetched reviews")
	}
	if ci[0].GitHub.Matches[0].Checks == nil || ci[0].GitHub.Matches[0].Review != nil {
		t.Fatal("CI-only response did not follow its selection")
	}
	prs := a.Metadata().Observe(t.Context(), rows, MetadataRequest{PRs: true})
	if prs[0].GitHub.Matches[0].Checks != nil || prs[0].GitHub.Matches[0].Review != nil {
		t.Fatal("PR-only response included unrequested features")
	}
}

func TestMetadataSelectionIsIndependentAndEmptyRequestDoesNoWork(t *testing.T) {
	checkedAt := time.Now().UTC()
	rows := []WorkspaceView{{GitHub: &PRInfo{CheckedAt: &checkedAt, Matches: []PullRequest{{State: "OPEN", Checks: &CheckSummary{Passed: 1}, Review: &ReviewResolution{Code: "old"}}}}}}
	selected := selectMetadata(rows, MetadataRequest{PRs: true, CI: true, Reviews: true})
	selected[0].GitHub.Matches[0].State = "CLOSED"
	selected[0].GitHub.Matches[0].Checks.Passed = 9
	selected[0].GitHub.Matches[0].Review.Code = "new"
	*selected[0].GitHub.CheckedAt = checkedAt.Add(time.Hour)
	if rows[0].GitHub.Matches[0].State != "OPEN" || rows[0].GitHub.Matches[0].Checks.Passed != 1 || rows[0].GitHub.Matches[0].Review.Code != "old" || !rows[0].GitHub.CheckedAt.Equal(checkedAt) {
		t.Fatal("selection changed an existing observation")
	}
	a := New(t.TempDir(), t.TempDir(), nil)
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		t.Fatal("empty request fetched provider metadata")
		return nil, nil
	}}
	observed := a.Metadata().Observe(t.Context(), rows, MetadataRequest{})
	if len(observed) != 1 || observed[0].GitHub != nil {
		t.Fatal("empty request retained provider metadata")
	}
}

func TestMetadataRefreshRetainsFailedObservationWithoutReviewRequests(t *testing.T) {
	a, r, _ := fixture(t)
	if _, err := output(t.Context(), r.Root, "git", "remote", "add", "origin", "https://github.com/owner/repo.git"); err != nil {
		t.Fatal(err)
	}
	rows, err := a.List(t.Context(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	a.PRs = fakePRClient{call: func(context.Context, string) ([]PullRequest, error) { return nil, Fail("offline", "offline") }}
	observed := a.Metadata().Observe(t.Context(), rows, MetadataRequest{PRs: true, Reviews: true})
	if observed[0].GitHub.Lookup != "unavailable" || observed[0].GitHub.Error == "" {
		t.Fatal("provider failure became successful empty association")
	}
}
