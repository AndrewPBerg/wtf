package core

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed review_bridge.mjs
var reviewBridgeSource []byte

// ReviewResolution identifies a verified review destination and its account scope.
type ReviewResolution struct {
	URL               string    `json:"url,omitempty"`
	Provider          string    `json:"provider"`
	Code              string    `json:"code"`
	Status            string    `json:"status,omitempty"`
	Profile           string    `json:"profile,omitempty"`
	Server            string    `json:"server,omitempty"`
	Workspace         string    `json:"workspace,omitempty"`
	WorkspaceID       string    `json:"workspace_id,omitempty"`
	ExpectedWorkspace string    `json:"expected_workspace,omitempty"`
	CheckedAt         time.Time `json:"checked_at"`
}

// ReviewClient resolves PRs and probes their repository's existing MCP connection.
type ReviewClient interface {
	Resolve(context.Context, string, string) ReviewResolution
	Diagnose(context.Context, string) ReviewResolution
}

// NativeReviewClient uses the embedded adapter and Codex-owned MCP OAuth.
type NativeReviewClient struct{}

// Resolve finds a verified Linear review or retains the GitHub destination.
func (NativeReviewClient) Resolve(ctx context.Context, root, prURL string) ReviewResolution {
	return runReviewBridge(ctx, root, prURL, false)
}

// Diagnose checks the configured Linear connection and account identity.
func (NativeReviewClient) Diagnose(ctx context.Context, root string) ReviewResolution {
	return runReviewBridge(ctx, root, "", true)
}

func runReviewBridge(ctx context.Context, root, prURL string, doctor bool) ReviewResolution {
	r := ReviewResolution{URL: prURL, Provider: "github", Code: "linear_unavailable", CheckedAt: time.Now().UTC()}
	if doctor {
		r.Provider, r.Status = "linear", "unavailable"
	}
	arguments := []string{"--pr", prURL}
	if doctor {
		arguments = []string{"--doctor", "true"}
	}
	data, runErr, code := invokeReviewAdapter(ctx, root, arguments, nil)
	if code != "" {
		r.Code = code
		return r
	}
	var result struct {
		OK                bool      `json:"ok"`
		Provider          string    `json:"provider"`
		URL               string    `json:"url"`
		Reason            string    `json:"reason"`
		Status            string    `json:"status"`
		Profile           string    `json:"profile"`
		Server            string    `json:"server"`
		CheckedAt         time.Time `json:"checkedAt"`
		ExpectedWorkspace string    `json:"expectedWorkspace"`
		Workspace         struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Reason == "" {
		r.Code = "invalid_review_adapter_response"
		return r
	}
	r.Code, r.Status = result.Reason, result.Status
	r.Profile, r.Server = result.Profile, result.Server
	r.Workspace, r.WorkspaceID = result.Workspace.Slug, result.Workspace.ID
	r.ExpectedWorkspace = result.ExpectedWorkspace
	if result.Status == "ready" && (!result.OK || runErr != nil) {
		r.Code, r.Status = "review_adapter_failed", "unavailable"
		return r
	}
	if !result.CheckedAt.IsZero() {
		r.CheckedAt = result.CheckedAt
	}
	if !doctor && result.OK && runErr == nil && result.Reason == "review_found" && result.Provider == "linear" && validLinearReview(result.URL, r.Workspace) && r.WorkspaceID != "" && r.Workspace == r.ExpectedWorkspace {
		r.URL, r.Provider = result.URL, "linear"
	}
	return r
}

func validLinearReview(raw, workspace string) bool {
	u, err := url.Parse(raw)
	return err == nil && workspace != "" && u.Scheme == "https" && u.Host == "linear.app" && u.User == nil &&
		strings.HasPrefix(u.Path, "/"+workspace+"/review/") && strings.IndexFunc(raw, func(r rune) bool { return r < 32 || r >= 127 && r <= 159 }) < 0
}

func (a *App) reviewClient() ReviewClient {
	if a.Reviews != nil {
		return a.Reviews
	}
	return NativeReviewClient{}
}

// WithReviewLinks augments matched PRs without replacing GitHub state or checks.
func (a *App) WithReviewLinks(ctx context.Context, rows []WorkspaceView, force bool) []WorkspaceView {
	// The browser may still be rendering the previous observation while refreshing.
	// Attach results to independent records rather than changing shared UI data.
	rows = append([]WorkspaceView{}, rows...)
	for i, row := range rows {
		if row.GitHub != nil {
			copy := *row.GitHub
			copy.Matches = append([]PullRequest{}, row.GitHub.Matches...)
			rows[i].GitHub = &copy
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cache := reviewCache{Version: 1, Entries: map[string]reviewCacheEntry{}}
	if err := readJSON(filepath.Join(a.Home, "review-links.json"), &cache); err != nil || cache.Version != 1 || cache.Entries == nil {
		cache = reviewCache{Version: 1, Entries: map[string]reviewCacheEntry{}}
	}
	changed := false
	updates := map[string]reviewCacheEntry{}
	invalidated := map[string]time.Time{}
	observations := map[string]reviewCacheEntry{}
	type target struct {
		key    string
		lookup ReviewLookup
	}
	pending := []target{}
	queued := map[string]bool{}
	roots := map[string]string{}
	scopes := map[string]string{}
	rowRoots := make([]string, len(rows))
	for index, row := range rows {
		if row.GitHub == nil || len(row.GitHub.Matches) == 0 {
			continue
		}
		root, known := roots[row.Path]
		if !known {
			root = row.RepositoryPath
			if root == "" {
				root = row.Path
				if repo, err := Discover(bounded, row.Path); err == nil {
					if state, err := repo.Load(); err == nil {
						root = state.Anchor
					}
				}
			}
			roots[row.Path] = root
		}
		rowRoots[index] = root
		scope, known := scopes[root]
		if !known {
			scope = reviewScope(root)
			scopes[root] = scope
		}
		for _, pr := range row.GitHub.Matches {
			key := scope + "\n" + pr.URL
			if _, exists := observations[key]; exists || queued[key] {
				continue
			}
			entry, exists := cache.Entries[key]
			if exists {
				verified := normalizeReview(root, pr.URL, entry.Result)
				if verified.Code != entry.Result.Code || verified.Provider != entry.Result.Provider {
					exists = false
				}
			}
			ttl := time.Minute
			if entry.Result.Provider == "linear" {
				ttl = 5 * time.Minute
			}
			age := time.Since(entry.Result.CheckedAt)
			if exists && !force && !entry.Result.CheckedAt.IsZero() && age >= 0 && age < ttl {
				observations[key] = entry
			} else {
				queued[key] = true
				pending = append(pending, target{key, ReviewLookup{Root: root, URL: pr.URL}})
			}
		}
	}
	store := func(item target, result ReviewResolution) {
		result = normalizeReview(item.lookup.Root, item.lookup.URL, result)
		entry := reviewCacheEntry{Result: result}
		observations[item.key] = entry
		if bounded.Err() == nil {
			changed = true
			if result.Code == "review_found" || result.Code == "review_not_found" {
				updates[item.key] = entry
			} else {
				invalidated[item.key] = time.Now().UTC()
			}
		}
	}
	client := a.reviewClient()
	if batch, ok := client.(ReviewBatchClient); ok {
		groups := map[string][]target{}
		order := []string{}
		for _, item := range pending {
			scope := reviewConnectionScope(item.lookup.Root)
			if _, exists := groups[scope]; !exists {
				order = append(order, scope)
			}
			groups[scope] = append(groups[scope], item)
		}
		// Compatible groups run sequentially: never more than one app-server.
		for _, scope := range order {
			items := groups[scope]
			lookups := make([]ReviewLookup, len(items))
			for i, item := range items {
				lookups[i] = item.lookup
			}
			results := batch.ResolveBatch(bounded, lookups)
			for i, item := range items {
				result := ReviewResolution{URL: item.lookup.URL, Provider: "github", Code: "invalid_review_batch_response", CheckedAt: time.Now().UTC()}
				if len(results) == len(items) {
					result = results[i]
				}
				store(item, result)
			}
		}
	} else {
		for _, item := range pending {
			store(item, client.Resolve(bounded, item.lookup.Root, item.lookup.URL))
		}
	}
	for index, row := range rows {
		if row.GitHub == nil {
			continue
		}
		root := rowRoots[index]
		for i, pr := range row.GitHub.Matches {
			result := normalizeReview(root, pr.URL, observations[scopes[root]+"\n"+pr.URL].Result)
			row.GitHub.Matches[i].Review = &result
		}
	}
	if changed && os.MkdirAll(a.Home, 0700) == nil {
		persist, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer done()
		if unlock, err := lock(persist, filepath.Join(a.Home, "review-links.lock")); err == nil {
			defer unlock()
			latest := reviewCache{Version: 1, Entries: map[string]reviewCacheEntry{}}
			if err := readJSON(filepath.Join(a.Home, "review-links.json"), &latest); err != nil || latest.Version != 1 || latest.Entries == nil {
				latest = reviewCache{Version: 1, Entries: map[string]reviewCacheEntry{}}
			}
			for key, update := range updates {
				previous, ok := latest.Entries[key]
				if !ok || previous.Result.CheckedAt.After(time.Now()) || !previous.Result.CheckedAt.After(update.Result.CheckedAt) {
					latest.Entries[key] = update
				}
			}
			for key, failedAt := range invalidated {
				if previous, ok := latest.Entries[key]; ok && !previous.Result.CheckedAt.After(failedAt) {
					delete(latest.Entries, key)
				}
			}
			_ = writeAtomic(filepath.Join(a.Home, "review-links.json"), latest)
		}
	}
	return rows
}

func normalizeReview(root, prURL string, result ReviewResolution) ReviewResolution {
	expected := result.ExpectedWorkspace
	if expected == "" {
		expected = expectedReviewWorkspace(root)
	}
	if result.Provider == "linear" && (!validLinearReview(result.URL, result.Workspace) || result.Workspace != expected || result.WorkspaceID == "") {
		result.Provider, result.Code = "github", "workspace_mismatch"
	}
	if result.Provider != "linear" {
		result.URL, result.Provider = prURL, "github"
		if result.Code == "review_found" {
			result.Code = "invalid_review_adapter_response"
		}
	}
	return result
}

type reviewCache struct {
	Version int                         `json:"version"`
	Entries map[string]reviewCacheEntry `json:"entries"`
}
type reviewCacheEntry struct {
	Result ReviewResolution `json:"result"`
}

func expectedReviewWorkspace(root string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		work := filepath.Join(home, "Desktop", "Work")
		if root == work || strings.HasPrefix(root, work+string(os.PathSeparator)) {
			return "alitahealth"
		}
	}
	return "cascadinglabs"
}

func reviewScope(root string) string {
	// Configuration metadata invalidates account routing without reading secrets.
	parts := []string{root, expectedReviewWorkspace(root)}
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"config.toml", "alita.config.toml", "cascading.config.toml"} {
			paths = append(paths, filepath.Join(home, ".codex", name))
		}
	}
	for parent := root; ; parent = filepath.Dir(parent) {
		paths = append(paths, filepath.Join(parent, ".codex", "config.toml"))
		if filepath.Dir(parent) == parent {
			break
		}
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			parts = append(parts, path+":"+info.ModTime().UTC().Format(time.RFC3339Nano))
		}
	}
	return strings.Join(parts, "\n")
}

func (a *App) linearToolCheck(ctx context.Context) ToolCheck {
	result := a.reviewClient().Diagnose(ctx, a.Cwd)
	check := ToolCheck{Tool: "linear", Code: result.Code, Status: result.Status, Feature: "Linear-first PR links", Review: &result}
	if result.Status == "ready" {
		check.Message = fmt.Sprintf("Verified %s through %s", result.Workspace, result.Server)
	} else {
		check.Status = "unavailable"
		check.Message = "Linear review lookup is unavailable; PR links fall back to GitHub (" + result.Code + ")"
		check.Remedy = "Check this repository's Codex profile and run codex mcp list --json; authenticate its selected Linear MCP connection if needed"
		switch result.Code {
		case "node_missing":
			check.Remedy = "Make Node.js available on PATH or through Mise for the embedded review adapter"
		case "codex_config_unavailable", "codex_start_failed":
			check.Remedy = "Make Codex available on PATH or through Mise and check codex mcp list --json in this repository"
		case "workspace_mismatch":
			check.Message = fmt.Sprintf("Expected %s; authenticated %s through %s. PR links fall back to GitHub", result.ExpectedWorkspace, result.Workspace, result.Server)
			check.Remedy = "Correct this repository's Codex profile/Linear connection; its authenticated workspace differs from the expected account"
		}
	}
	return check
}
