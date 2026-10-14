package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReviewLookup identifies an exact PR and its repository's account configuration.
type ReviewLookup struct {
	Root string `json:"root"`
	URL  string `json:"url"`
}

// ReviewBatchClient resolves a compatible account group with bounded async work.
// Results correspond to input order. Providers retain authentication ownership.
type ReviewBatchClient interface {
	ResolveBatch(context.Context, []ReviewLookup) []ReviewResolution
}

// ResolveBatch uses one app-server, at most two verified ephemeral contexts and
// four outstanding calls per context. It never starts a model turn or retries.
func (NativeReviewClient) ResolveBatch(ctx context.Context, lookups []ReviewLookup) []ReviewResolution {
	out := make([]ReviewResolution, len(lookups))
	fallback := func(code string) []ReviewResolution {
		for i, lookup := range lookups {
			out[i] = ReviewResolution{URL: lookup.URL, Provider: "github", Code: code, CheckedAt: time.Now().UTC()}
		}
		return out
	}
	if len(lookups) == 0 {
		return out
	}
	if len(lookups) > 200 {
		return fallback("review_batch_limit")
	}
	urls, roots := []string{}, []string{}
	seen := map[string]bool{}
	for _, lookup := range lookups {
		urls = append(urls, lookup.URL)
		if !seen[lookup.Root] {
			roots = append(roots, lookup.Root)
			seen[lookup.Root] = true
		}
	}
	input, err := json.Marshal(map[string]any{"urls": urls, "roots": roots})
	if err != nil {
		return fallback("invalid_review_batch")
	}
	data, code, runErr := invokeReviewAdapter(ctx, lookups[0].Root, []string{"--batch", "true"}, bytes.NewReader(input))
	if code != "" {
		return fallback(code)
	}
	var result struct {
		OK                bool   `json:"ok"`
		Reason            string `json:"reason"`
		Profile           string `json:"profile"`
		Server            string `json:"server"`
		ExpectedWorkspace string `json:"expectedWorkspace"`
		Workspace         struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"workspace"`
		Results []struct {
			OK        bool      `json:"ok"`
			GitHubURL string    `json:"githubUrl"`
			URL       string    `json:"url"`
			Provider  string    `json:"provider"`
			Reason    string    `json:"reason"`
			CheckedAt time.Time `json:"checkedAt"`
		} `json:"results"`
	}
	if json.Unmarshal(data, &result) != nil || runErr != nil || !result.OK || result.Reason == "" {
		return fallback("invalid_review_adapter_response")
	}
	if result.Reason != "batch_resolved" {
		return fallback(result.Reason)
	}
	if len(result.Results) != len(lookups) {
		return fallback("invalid_review_adapter_response")
	}
	for i, value := range result.Results {
		if value.GitHubURL != lookups[i].URL || value.Reason == "" {
			return fallback("invalid_review_adapter_response")
		}
		r := ReviewResolution{URL: lookups[i].URL, Provider: "github", Code: value.Reason, Profile: result.Profile, Server: result.Server, Workspace: result.Workspace.Slug, WorkspaceID: result.Workspace.ID, ExpectedWorkspace: result.ExpectedWorkspace, CheckedAt: value.CheckedAt}
		if r.CheckedAt.IsZero() {
			r.CheckedAt = time.Now().UTC()
		}
		if value.Reason == "review_found" {
			if value.OK && value.Provider == "linear" && validLinearReview(value.URL, r.Workspace) && r.WorkspaceID != "" && r.Workspace == r.ExpectedWorkspace {
				r.URL, r.Provider = value.URL, "linear"
			} else {
				r.Code = "invalid_review_adapter_response"
			}
		}
		out[i] = r
	}
	return out
}

// Cache ownership stays repository-specific; transport reuse also requires the
// same expected account and configuration file identities. The bridge checks
// actual connection selection independently for every distinct root.
func reviewConnectionScope(root string) string {
	_, scope, _ := strings.Cut(reviewScope(root), "\n")
	profile := "plain"
	if home, err := os.UserHomeDir(); err == nil {
		for _, item := range []struct{ directory, profile string }{{"Work", "alita"}, {"cl", "cascading"}} {
			parent := filepath.Join(home, "Desktop", item.directory)
			if root == parent || strings.HasPrefix(root, parent+string(os.PathSeparator)) {
				profile = item.profile
			}
		}
	}
	return profile + "\n" + scope
}
