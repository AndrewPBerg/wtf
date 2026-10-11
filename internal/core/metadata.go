package core

import "context"

// MetadataRequest selects host-managed observations. CI and Reviews both depend
// on PR association; callers never need to fetch those dependencies themselves.
// ForceReviews bypasses the review-link cache, not account verification.
type MetadataRequest struct {
	PRs          bool `json:"prs"`
	CI           bool `json:"ci"`
	Reviews      bool `json:"reviews"`
	ForceReviews bool `json:"force_reviews"`
}

func (r MetadataRequest) needsPRs() bool { return r.PRs || r.CI || r.Reviews }

// MetadataService is the shared observation API for commands and future Lua
// bindings. Provider transport, grouping, caches and deadlines stay in the host.
type MetadataService struct{ app *App }

// Metadata creates a command-owned metadata service.
func (a *App) Metadata() *MetadataService { return &MetadataService{app: a} }

// MetadataPlan contains a cache-first view and the prepared repository identities.
// Refresh publishes a separate observation, preserving any rendered preview.
type MetadataPlan struct {
	Cached  []WorkspaceView
	service *MetadataService
	request MetadataRequest
	prs     *PRService
	plan    PRPlan
}

// Prepare reads local identities and cached PR data without provider requests.
func (s *MetadataService) Prepare(ctx context.Context, rows []WorkspaceView, request MetadataRequest) MetadataPlan {
	prepared := MetadataPlan{service: s, request: request}
	if !request.needsPRs() {
		prepared.Cached = selectMetadata(rows, request)
		return prepared
	}
	prepared.prs = s.app.GitHubService()
	prepared.plan = prepared.prs.Prepare(ctx, rows, false)
	prepared.Cached = selectMetadata(prepared.plan.Rows, request)
	return prepared
}

// Observe fetches one fresh repository observation shared by all workspace rows.
// Review links retain their own cache policy unless ForceReviews is requested.
func (s *MetadataService) Observe(ctx context.Context, rows []WorkspaceView, request MetadataRequest) []WorkspaceView {
	return s.Prepare(ctx, rows, request).Refresh(ctx)
}

// Refresh requests live GitHub state even when the preview cache is recent.
// Failures retain previous observations with the provider's unavailable/stale
// status; they never become a successful empty association.
func (p MetadataPlan) Refresh(ctx context.Context) []WorkspaceView {
	if p.prs == nil {
		return selectMetadata(p.Cached, p.request)
	}
	plan := p.plan
	plan.Groups = append([]prGroup{}, plan.Groups...)
	plan.NeedsRefresh = false
	for i := range plan.Groups {
		group := &plan.Groups[i]
		group.Refresh = group.Repository != "" && group.Error == ""
		plan.NeedsRefresh = plan.NeedsRefresh || group.Refresh
	}
	rows := plan.Rows
	if plan.NeedsRefresh {
		rows = p.prs.Refresh(ctx, plan)
	}
	if p.request.Reviews {
		rows = p.service.app.WithReviewLinks(ctx, rows, p.request.ForceReviews)
	}
	return selectMetadata(rows, p.request)
}

func selectMetadata(rows []WorkspaceView, request MetadataRequest) []WorkspaceView {
	result := append([]WorkspaceView{}, rows...)
	for i, row := range rows {
		if !request.needsPRs() || row.GitHub == nil {
			result[i].GitHub = nil
			continue
		}
		info := *row.GitHub
		info.Matches = append([]PullRequest{}, info.Matches...)
		for j := range info.Matches {
			pr := &info.Matches[j]
			pr.Rollup = nil // Raw provider payloads are not part of the host API.
			if request.CI && pr.Checks != nil {
				checks := *pr.Checks
				pr.Checks = &checks
			} else {
				pr.Checks = nil
			}
			if request.Reviews && pr.Review != nil {
				review := *pr.Review
				pr.Review = &review
			} else {
				pr.Review = nil
			}
		}
		if info.CheckedAt != nil {
			checkedAt := *info.CheckedAt
			info.CheckedAt = &checkedAt
		}
		result[i].GitHub = &info
	}
	return result
}
