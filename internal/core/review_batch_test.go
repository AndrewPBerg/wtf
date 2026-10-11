package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type fakeReviewBatch struct {
	fakeReviewClient
	batch func(context.Context, []ReviewLookup) []ReviewResolution
}

func (f fakeReviewBatch) ResolveBatch(ctx context.Context, lookups []ReviewLookup) []ReviewResolution {
	return f.batch(ctx, lookups)
}

func TestReviewBatchCollectsDistinctPRsSharesConnectionAndPreservesCacheOwnership(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	rootA, rootB := filepath.Join(a.Cwd, "one"), filepath.Join(a.Cwd, "two")
	calls := 0
	a.Reviews = fakeReviewBatch{batch: func(_ context.Context, lookups []ReviewLookup) []ReviewResolution {
		calls++
		if len(lookups) != 3 {
			t.Fatalf("batch contains %d lookups; wanted distinct PRs and separate repository cache ownership", len(lookups))
		}
		results := make([]ReviewResolution, len(lookups))
		for i := range results {
			results[i] = ReviewResolution{URL: lookups[i].URL, Provider: "github", Code: "review_not_found", CheckedAt: time.Now().UTC()}
		}
		return results
	}}
	row := func(root string, numbers ...int) WorkspaceView {
		matches := []PullRequest{}
		for _, number := range numbers {
			matches = append(matches, PullRequest{Number: number, URL: "https://github.com/owner/repo/pull/" + strconv.Itoa(number), Checks: &CheckSummary{Passed: 2}})
		}
		return WorkspaceView{RepositoryPath: root, Workspace: Workspace{Path: root}, GitHub: &PRInfo{Matches: matches}}
	}
	rows := []WorkspaceView{row(rootA, 1, 2), row(rootA, 1), row(rootB, 1)}
	observed := a.WithReviewLinks(t.Context(), rows, true)
	if calls != 1 {
		t.Fatal("compatible roots did not share a provider batch")
	}
	for _, row := range observed {
		for _, pr := range row.GitHub.Matches {
			if pr.Review == nil || pr.Review.Code != "review_not_found" || pr.Review.URL != pr.URL || pr.Checks.Passed != 2 {
				t.Fatal("batch result lost association, fallback or CI")
			}
		}
	}
	a.WithReviewLinks(t.Context(), rows, false)
	if calls != 1 {
		t.Fatal("batch bypassed the review cache")
	}
}

func TestReviewConnectionScopeSeparatesProjectConfigurationAndAccounts(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "one"), filepath.Join(root, "two")
	if reviewConnectionScope(first) != reviewConnectionScope(second) {
		t.Fatal("compatible roots cannot reuse a connection")
	}
	if err := os.MkdirAll(filepath.Join(second, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, ".codex", "config.toml"), []byte("# project configuration\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if reviewConnectionScope(first) == reviewConnectionScope(second) {
		t.Fatal("different project configurations shared a connection")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if reviewConnectionScope(filepath.Join(home, "Desktop", "Work", "fixture")) == reviewConnectionScope(filepath.Join(home, "Desktop", "cl", "fixture")) {
		t.Fatal("different expected accounts shared a connection")
	}
}

func TestReviewBatchInvalidResponseIsTransientAndDoesNotInventMissingReviews(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	calls := 0
	a.Reviews = fakeReviewBatch{batch: func(context.Context, []ReviewLookup) []ReviewResolution { calls++; return nil }}
	rows := reviewTestRows(a.Cwd)
	first := a.WithReviewLinks(t.Context(), rows, true)
	if first[0].GitHub.Matches[0].Review.Code != "invalid_review_batch_response" {
		t.Fatal("invalid batch became a confirmed missing review")
	}
	a.WithReviewLinks(t.Context(), rows, false)
	if calls != 2 {
		t.Fatal("invalid response was cached")
	}
}

func TestReviewBatchDoesNotCacheInvalidVerifiedDestinations(t *testing.T) {
	a := New(t.TempDir(), t.TempDir(), nil)
	calls := 0
	a.Reviews = fakeReviewBatch{batch: func(context.Context, []ReviewLookup) []ReviewResolution {
		calls++
		return []ReviewResolution{{Provider: "linear", Code: "review_found", URL: "https://linear.app/alitahealth/review/wrong", Workspace: "alitahealth", WorkspaceID: "wrong", CheckedAt: time.Now().UTC()}}
	}}
	rows := reviewTestRows(a.Cwd)
	first := a.WithReviewLinks(t.Context(), rows, true)
	if first[0].GitHub.Matches[0].Review.Provider != "github" || first[0].GitHub.Matches[0].Review.Code != "workspace_mismatch" {
		t.Fatal("invalid destination was trusted")
	}
	a.WithReviewLinks(t.Context(), rows, false)
	if calls != 2 {
		t.Fatal("invalid destination was cached as verified")
	}
}

func TestNativeReviewBatchRejectsOversizedAndCancelledWorkBeforeLaunching(t *testing.T) {
	client := NativeReviewClient{}
	large := make([]ReviewLookup, 201)
	if results := client.ResolveBatch(t.Context(), large); len(results) != len(large) || results[0].Code != "review_batch_limit" {
		t.Fatal("oversized batch launched or lost results")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	input := []ReviewLookup{{Root: t.TempDir(), URL: "https://github.com/owner/repo/pull/1"}}
	if result := client.ResolveBatch(ctx, input); result[0].Code != "review_lookup_cancelled" || result[0].URL != input[0].URL {
		t.Fatal("cancelled batch launched or lost the fallback URL")
	}
}

func TestNativeReviewBatchLive(t *testing.T) {
	value := os.Getenv("WTF_REVIEW_BATCH_LIVE_CASES")
	if value == "" {
		t.Skip("set WTF_REVIEW_BATCH_LIVE_CASES for authorized live batch checks")
	}
	var lookups []ReviewLookup
	if err := json.Unmarshal([]byte(value), &lookups); err != nil {
		t.Fatal(err)
	}
	results := (NativeReviewClient{}).ResolveBatch(t.Context(), lookups)
	for i, result := range results {
		if result.Provider != "linear" || result.Code != "review_found" || result.Workspace != "alitahealth" || !validLinearReview(result.URL, result.Workspace) {
			t.Fatalf("batch item %d did not resolve through the expected account: %s", i, result.Code)
		}
	}
}
