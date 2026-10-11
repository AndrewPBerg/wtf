package core

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
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

func (NativeReviewClient) Resolve(ctx context.Context, root, prURL string) ReviewResolution {
	return runReviewBridge(ctx, root, prURL, false)
}

func (NativeReviewClient) Diagnose(ctx context.Context, root string) ReviewResolution {
	return runReviewBridge(ctx, root, "", true)
}

func runReviewBridge(ctx context.Context, root, prURL string, doctor bool) ReviewResolution {
	r := ReviewResolution{URL: prURL, Provider: "github", Code: "linear_unavailable", CheckedAt: time.Now().UTC()}
	if doctor {
		r.Provider, r.Status = "linear", "unavailable"
	}
	if ctx.Err() != nil {
		r.Code = "review_lookup_cancelled"
		return r
	}
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	node, err := doctorBinary(bounded, root, "node")
	if err != nil {
		r.Code = "node_missing"
		return r
	}
	file, err := os.CreateTemp("", "wtf-review-*.mjs")
	if err != nil {
		r.Code = "review_adapter_unavailable"
		return r
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(reviewBridgeSource)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		r.Code = "review_adapter_unavailable"
		return r
	}
	args := []string{file.Name(), "--repo", root, "--timeout-ms", "5000"}
	if doctor {
		args = append(args, "--doctor", "true")
	} else {
		args = append(args, "--pr", prURL)
	}
	cmd := exec.CommandContext(bounded, node, args...)
	cmd.Dir = root
	cmd.Stdin = nil
	var data bytes.Buffer
	cmd.Stdout, cmd.Stderr = &data, io.Discard
	configureCommand(cmd)
	runErr := cmd.Run() // Failed diagnostics still return a structured JSON result.
	finishCommand(cmd)
	if bounded.Err() != nil {
		r.Code = "linear_lookup_timeout"
		if ctx.Err() == context.Canceled {
			r.Code = "review_lookup_cancelled"
		}
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
	if err := json.Unmarshal(data.Bytes(), &result); err != nil || result.Reason == "" {
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
	for _, row := range rows {
		if row.GitHub == nil || len(row.GitHub.Matches) == 0 {
			continue
		}
		root := row.Path
		if repo, err := Discover(bounded, row.Path); err == nil {
			if state, err := repo.Load(); err == nil {
				root = state.Anchor
			}
		}
		for i, pr := range row.GitHub.Matches {
			key := reviewScope(root) + "\n" + pr.URL
			entry, exists := cache.Entries[key]
			ttl := time.Minute
			if entry.Result.Provider == "linear" {
				ttl = 5 * time.Minute
			}
			age := time.Since(entry.Result.CheckedAt)
			if !exists || force || entry.Result.CheckedAt.IsZero() || age < 0 || age >= ttl {
				entry = reviewCacheEntry{Result: a.reviewClient().Resolve(bounded, root, pr.URL)}
				if bounded.Err() == nil {
					changed = true
					if entry.Result.Code == "review_found" || entry.Result.Code == "review_not_found" {
						cache.Entries[key], updates[key] = entry, entry
					} else {
						delete(cache.Entries, key)
						invalidated[key] = time.Now().UTC()
					}
				}
			}
			result := entry.Result
			expected := result.ExpectedWorkspace
			if expected == "" {
				expected = expectedReviewWorkspace(root)
			}
			// Never attach a cached destination from the wrong configured account.
			if result.Provider == "linear" && (!validLinearReview(result.URL, result.Workspace) || result.Workspace != expected || result.WorkspaceID == "") {
				result.URL, result.Provider, result.Code = pr.URL, "github", "workspace_mismatch"
			}
			if result.Provider != "linear" {
				result.URL = pr.URL
			}
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
