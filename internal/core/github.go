package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const prTTL = 2 * time.Minute
const prLimit = 200
const prWorkers = 3

// PullRequest contains GitHub state, checks and its resolved review link.
type PullRequest struct {
	Number          int    `json:"number"`
	Title           string `json:"title"`
	URL             string `json:"url"`
	State           string `json:"state"`
	Draft           bool   `json:"isDraft"`
	Branch          string `json:"headRefName"`
	Head            string `json:"headRefOid"`
	CrossRepository bool   `json:"isCrossRepository"`
	Owner           struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	Relation string            `json:"relation,omitempty"`
	Rollup   json.RawMessage   `json:"statusCheckRollup,omitempty"`
	Checks   *CheckSummary     `json:"checks,omitempty"`
	Review   *ReviewResolution `json:"review,omitempty"`
}

// CheckSummary describes GitHub checks on the published PR head, not local work.
type CheckSummary struct {
	Head      string `json:"head"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`
	Pending   int    `json:"pending"`
	Passed    int    `json:"passed"`
	Skipped   int    `json:"skipped"`
	Cancelled int    `json:"cancelled"`
	Unknown   int    `json:"unknown"`
	Truncated bool   `json:"truncated,omitempty"`
}

func summarizeChecks(head string, raw json.RawMessage) (*CheckSummary, error) {
	if len(raw) == 0 {
		return nil, nil // Older cache entries have no check observation.
	}
	var checks []struct {
		Type       string `json:"__typename"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	}
	if err := json.Unmarshal(raw, &checks); err != nil {
		return nil, Fail("invalid_github_response", "GitHub returned invalid check data")
	}
	summary := &CheckSummary{Head: head, Total: len(checks), Truncated: len(checks) >= 100}
	for _, check := range checks {
		if check.Type != "CheckRun" && check.Type != "StatusContext" {
			summary.Unknown++
			continue
		}
		state := check.State
		if check.Type == "CheckRun" {
			if check.Status != "COMPLETED" {
				state = check.Status
			} else {
				state = check.Conclusion
			}
		}
		switch state {
		case "FAILURE", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			summary.Failed++
		case "QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "REQUESTED", "EXPECTED":
			summary.Pending++
		case "SUCCESS":
			summary.Passed++
		case "SKIPPED", "NEUTRAL":
			summary.Skipped++
		case "CANCELLED":
			summary.Cancelled++
		default:
			summary.Unknown++
		}
	}
	return summary, nil
}

// PRInfo is a workspace PR observation, including lookup failures.
type PRInfo struct {
	Repository string        `json:"repository,omitempty"`
	Lookup     string        `json:"lookup"`
	CheckedAt  *time.Time    `json:"checked_at,omitempty"`
	Matches    []PullRequest `json:"matches"`
	Truncated  bool          `json:"truncated,omitempty"`
	Error      string        `json:"error,omitempty"`
}
type prRevision struct {
	Commit    string   `json:"commit"`
	Bookmarks []string `json:"bookmarks"`
}
type prSnapshot struct {
	Name      string       `json:"name"`
	Commit    string       `json:"commit"`
	Empty     bool         `json:"empty"`
	Bookmarks []string     `json:"bookmarks"`
	Parents   []prRevision `json:"parents"`
}
type prCacheEntry struct {
	Repository string        `json:"repository"`
	CheckedAt  time.Time     `json:"checked_at"`
	PRs        []PullRequest `json:"prs"`
}
type prCache struct {
	Version      int                     `json:"version"`
	Repositories map[string]prCacheEntry `json:"repositories"`
}

// PRClient fetches pull requests for a GitHub repository.
type PRClient interface {
	List(context.Context, string) ([]PullRequest, error)
}

// GHClient fetches PR metadata through the installed GitHub CLI.
type GHClient struct{ Binary string }

// List fetches PR state and checks for the repository.
func (client GHClient) List(ctx context.Context, repository string) ([]PullRequest, error) {
	binary := client.Binary
	if binary == "" {
		binary = "gh"
		// Resolve an installed Mise tool without executing launchers that run setup
		// or print banners into the JSON stream. This does not install or configure it.
		if path, err := exec.CommandContext(ctx, "mise", "which", "gh").Output(); err == nil {
			candidate := strings.TrimSpace(string(path))
			if filepath.IsAbs(candidate) {
				if stat, err := os.Stat(candidate); err == nil && stat.Mode().IsRegular() && stat.Mode().Perm()&0111 != 0 {
					binary = candidate
				}
			}
		}
	}
	cmd := exec.CommandContext(ctx, binary, "pr", "list", "--repo", repository, "--state", "all", "--limit", "200", "--json", "number,title,url,state,isDraft,headRefName,headRefOid,headRepositoryOwner,isCrossRepository,statusCheckRollup")
	var data bytes.Buffer
	cmd.Stdout = &data
	cmd.Stderr = io.Discard
	configureCommand(cmd)
	if err := cmd.Run(); err != nil {
		finishCommand(cmd)
		return nil, Fail("github_unavailable", "GitHub lookup failed; check gh authentication or network access")
	}
	finishCommand(cmd)
	var prs []PullRequest
	if err := json.Unmarshal(data.Bytes(), &prs); err != nil {
		return nil, Fail("invalid_github_response", "GitHub returned an invalid PR list")
	}
	for i := range prs {
		checks, err := summarizeChecks(prs[i].Head, prs[i].Rollup)
		if err != nil {
			return nil, err
		}
		prs[i].Checks = checks
	}
	return prs, nil
}

type prGroup struct {
	ID, Repository, Root string
	Snapshot             map[string]prSnapshot
	Cached               *prCacheEntry
	Refresh              bool
	Error                string
}

// PRPlan groups workspace observations for a bounded refresh.
type PRPlan struct {
	Groups       []prGroup
	Rows         []WorkspaceView
	NeedsRefresh bool
}

// PRService loads, associates and caches GitHub PR observations.
type PRService struct {
	Home   string
	Client PRClient
	Now    func() time.Time
	Log    io.Writer
}

// GitHubService constructs the repository PR service.
func (a *App) GitHubService() *PRService { return &PRService{a.Home, GHClient{}, time.Now, a.Out} }

var githubSlug = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func githubRepository(remote string) string {
	var path string
	if strings.HasPrefix(remote, "git@github.com:") {
		path = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		u, e := url.Parse(remote)
		if e != nil || !strings.EqualFold(u.Hostname(), "github.com") {
			return ""
		}
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	if !githubSlug.MatchString(path) {
		return ""
	}
	return path
}
func repoGitHub(ctx context.Context, root string) (string, error) {
	v, e := output(ctx, root, "git", "remote", "list", "--ignore-working-copy")
	if e != nil {
		return "", e
	}
	var fallback string
	for _, line := range strings.Split(v, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		repo := githubRepository(parts[1])
		if repo == "" {
			continue
		}
		if parts[0] == "origin" {
			return repo, nil
		}
		if fallback == "" {
			fallback = repo
		}
	}
	return fallback, nil
}

const prSnapshotTemplate = `"{\"name\":" ++ json(name) ++ ",\"commit\":" ++ json(target.commit_id()) ++ ",\"empty\":" ++ json(target.empty()) ++ ",\"bookmarks\":" ++ json(target.local_bookmarks().map(|b| b.name())) ++ ",\"parents\":[" ++ target.parents().map(|p| "{\"commit\":" ++ json(p.commit_id()) ++ ",\"bookmarks\":" ++ json(p.local_bookmarks().map(|b| b.name())) ++ "}").join(",") ++ "]}\n"`

func snapshots(ctx context.Context, root string) (map[string]prSnapshot, error) {
	v, e := output(ctx, root, "workspace", "list", "--ignore-working-copy", "-T", prSnapshotTemplate)
	if e != nil {
		return nil, e
	}
	result := map[string]prSnapshot{}
	for _, line := range strings.Split(v, "\n") {
		if line == "" {
			continue
		}
		var row prSnapshot
		if e = json.Unmarshal([]byte(line), &row); e != nil {
			return nil, Fail("jj_failed", "Cannot decode JJ workspace revisions")
		}
		result[row.Name] = row
	}
	return result, nil
}
func (s *PRService) cache() prCache {
	c := prCache{1, map[string]prCacheEntry{}}
	if readJSON(filepath.Join(s.Home, "github-pr-cache.json"), &c) != nil || c.Version != 1 || c.Repositories == nil {
		return prCache{1, map[string]prCacheEntry{}}
	}
	return c
}
func (s *PRService) save(ctx context.Context, updates map[string]prCacheEntry) error {
	if len(updates) == 0 {
		return nil
	}
	if e := os.MkdirAll(s.Home, 0700); e != nil {
		return e
	}
	unlock, e := lock(ctx, filepath.Join(s.Home, "github-pr-cache.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	c := s.cache()
	for id, entry := range updates {
		if old, ok := c.Repositories[id]; !ok || !old.CheckedAt.After(entry.CheckedAt) {
			c.Repositories[id] = entry
		}
	}
	return writeAtomic(filepath.Join(s.Home, "github-pr-cache.json"), c)
}

// Prepare plans a refresh using local identity and cached observations.
func (s *PRService) Prepare(ctx context.Context, rows []WorkspaceView, force bool) PRPlan {
	plan := PRPlan{Rows: append([]WorkspaceView{}, rows...)}
	groups := map[string]*prGroup{}
	cache := s.cache()
	localCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, row := range rows {
		if row.Status == "removed" || !row.Exists {
			continue
		}
		if _, ok := groups[row.RepositoryID]; !ok {
			groups[row.RepositoryID] = &prGroup{ID: row.RepositoryID, Root: row.Path}
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g := groups[id]
		var e error
		g.Repository, e = repoGitHub(localCtx, g.Root)
		if e != nil {
			g.Error = "JJ remote inspection failed"
		} else if g.Repository != "" {
			g.Snapshot, e = snapshots(localCtx, g.Root)
			if e != nil {
				g.Error = "JJ revision inspection failed"
			}
		}
		if entry, ok := cache.Repositories[id]; ok && entry.Repository == g.Repository {
			copy := entry
			g.Cached = &copy
		}
		g.Refresh = g.Repository != "" && g.Error == "" && (force || g.Cached == nil || s.Now().Sub(g.Cached.CheckedAt) > prTTL)
		plan.NeedsRefresh = plan.NeedsRefresh || g.Refresh
		plan.Groups = append(plan.Groups, *g)
	}
	plan.Rows = applyPRs(plan.Rows, plan.Groups, s.Now(), false)
	return plan
}
func associate(snapshot prSnapshot, prs []PullRequest, repository string) []PullRequest {
	matches := []PullRequest{}
	branches := map[string]bool{}
	for _, name := range snapshot.Bookmarks {
		branches[name] = true
	}
	for _, parent := range snapshot.Parents {
		for _, name := range parent.Bookmarks {
			branches[name] = true
		}
	}
	owner, _, _ := strings.Cut(repository, "/")
	for _, pr := range prs {
		relation := ""
		if snapshot.Commit == pr.Head {
			relation = "head_matches"
		} else {
			for _, parent := range snapshot.Parents {
				if parent.Commit == pr.Head {
					relation = "local_differs"
					if snapshot.Empty {
						relation = "base_matches"
					}
					break
				}
			}
		}
		branchMatches := branches[pr.Branch] && (!pr.CrossRepository || strings.EqualFold(pr.Owner.Login, owner))
		if relation == "" && !branchMatches {
			continue
		}
		if relation == "" {
			relation = "local_differs"
		}
		pr.Relation = relation
		matches = append(matches, pr)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if (matches[i].State == "OPEN") != (matches[j].State == "OPEN") {
			return matches[i].State == "OPEN"
		}
		return matches[i].Number > matches[j].Number
	})
	return matches
}
func applyPRs(rows []WorkspaceView, groups []prGroup, now time.Time, finished bool) []WorkspaceView {
	out := append([]WorkspaceView{}, rows...)
	byID := map[string]prGroup{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	for i, row := range out {
		info := &PRInfo{Lookup: "unavailable", Matches: []PullRequest{}}
		out[i].GitHub = info
		if row.Status == "removed" || !row.Exists {
			info.Lookup = "not_inspected"
			continue
		}
		g, ok := byID[row.RepositoryID]
		if !ok {
			continue
		}
		info.Repository = g.Repository
		if g.Error != "" {
			info.Error = g.Error
		} else if g.Repository == "" {
			info.Lookup = "not_github"
			continue
		}
		if g.Cached != nil {
			info.CheckedAt = &g.Cached.CheckedAt
			info.Truncated = len(g.Cached.PRs) >= prLimit
			info.Lookup = "cached"
			if now.Sub(g.Cached.CheckedAt) > prTTL {
				info.Lookup = "stale"
			}
			if snap, ok := g.Snapshot[row.Name]; ok {
				info.Matches = associate(snap, g.Cached.PRs, g.Repository)
			} else {
				info.Lookup = "unavailable"
				info.Error = "Native JJ workspace not found"
			}
		}
		if g.Refresh && !finished {
			if g.Cached == nil {
				info.Lookup = "loading"
			}
		} else if g.Refresh && g.Error == "" {
			info.Lookup = "fresh"
		}
	}
	return out
}

// Refresh fetches planned PR observations with bounded concurrency.
func (s *PRService) Refresh(ctx context.Context, plan PRPlan) []WorkspaceView {
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type response struct {
		Index int
		PRs   []PullRequest
		Error error
	}
	jobs := make(chan int, len(plan.Groups))
	results := make(chan response, len(plan.Groups))
	count := 0
	for i, g := range plan.Groups {
		if g.Refresh {
			jobs <- i
			count++
		}
	}
	close(jobs)
	var wg sync.WaitGroup
	for worker := 0; worker < min(prWorkers, count); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				prs, e := s.Client.List(bounded, plan.Groups[i].Repository)
				results <- response{i, prs, e}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	updates := map[string]prCacheEntry{}
	for result := range results {
		g := &plan.Groups[result.Index]
		if result.Error != nil {
			g.Error = result.Error.Error()
			continue
		}
		entry := prCacheEntry{g.Repository, s.Now().UTC(), result.PRs}
		g.Cached = &entry
		updates[g.ID] = entry
	}
	persist, done := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer done()
	if e := s.save(persist, updates); e != nil && s.Log != nil {
		_, _ = io.WriteString(s.Log, "PR cache could not be saved; live results are still shown.\n")
	}
	return applyPRs(plan.Rows, plan.Groups, s.Now(), true)
}
