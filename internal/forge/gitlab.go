package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// gitLab implements Forge for GitLab repositories.
type gitLab struct {
	host      string // e.g. "gitlab.com"
	owner     string
	repo      string
	token     string
	apiURL    string // e.g. "https://gitlab.com/api/v4"
	projectID string // URL-encoded "owner/repo"
	client    *http.Client
}

func newGitLab(host, owner, repo string, tokenFn TokenFunc) (*gitLab, error) {
	token, err := tokenFn()
	if err != nil {
		return nil, fmt.Errorf("getting gitlab token: %w", err)
	}

	return &gitLab{
		host:      host,
		owner:     owner,
		repo:      repo,
		token:     token,
		apiURL:    "https://" + host + "/api/v4",
		projectID: url.PathEscape(owner + "/" + repo),
		client:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (g *gitLab) Name() string { return "gitlab" }

func (g *gitLab) PRURL(number int) string {
	return fmt.Sprintf("https://%s/%s/%s/-/merge_requests/%d", g.host, g.owner, g.repo, number)
}

func (g *gitLab) FetchRef(number int) string {
	return fmt.Sprintf("merge-requests/%d/head:mr-%d", number, number)
}

// glMR is the JSON shape returned by the GitLab REST API for merge requests.
type glMR struct {
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"` // "opened", "closed", "merged"
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	Draft     bool   `json:"draft"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
	SourceBranch string `json:"source_branch"`
}

func (g *gitLab) ListPRs(ctx context.Context) ([]PR, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests?state=opened&per_page=100",
		g.apiURL, g.projectID)

	body, err := g.doGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("listing MRs: %w", err)
	}

	var mrs []glMR
	if err := json.Unmarshal(body, &mrs); err != nil {
		return nil, fmt.Errorf("parsing MR list: %w", err)
	}

	prs := make([]PR, len(mrs))
	for i, mr := range mrs {
		prs[i] = g.toPR(mr)
	}
	return prs, nil
}

func (g *gitLab) GetPR(ctx context.Context, number int) (*PR, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d",
		g.apiURL, g.projectID, number)

	body, err := g.doGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("getting MR !%d: %w", number, err)
	}

	var mr glMR
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("parsing MR !%d: %w", number, err)
	}

	pr := g.toPR(mr)
	return &pr, nil
}

func (g *gitLab) toPR(mr glMR) PR {
	created, _ := time.Parse(time.RFC3339, mr.CreatedAt)
	return PR{
		Number:    mr.IID,
		Title:     mr.Title,
		Branch:    mr.SourceBranch,
		Author:    mr.Author.Username,
		CreatedAt: created,
		URL:       mr.WebURL,
		IsDraft:   mr.Draft,
		State:     glState(mr),
	}
}

// glState maps GitLab MR state to PRState.
func glState(mr glMR) PRState {
	switch mr.State {
	case "merged":
		return PRMerged
	case "closed":
		return PRClosed
	default:
		return PROpen
	}
}

func (g *gitLab) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("API returned 401 Unauthorized — check your token or run 'glab auth login' to re-authenticate")
		case http.StatusForbidden:
			return nil, fmt.Errorf("API returned 403 Forbidden — your token may lack the required scope, or try 'glab auth login' to switch accounts")
		case http.StatusNotFound:
			if g.token == "" {
				return nil, fmt.Errorf("API returned 404 Not Found — no token provided, run 'glab auth login'")
			}
			return nil, fmt.Errorf("API returned 404 Not Found — project may be private, check you're authenticated as the right user with 'glab auth status'")
		default:
			return nil, fmt.Errorf("API returned %s", resp.Status)
		}
	}

	buf, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return buf, nil
}

// glabToken retrieves a GitLab token via the glab CLI or GITLAB_TOKEN env var.
func glabToken() (string, error) {
	// Check env var first
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		return token, nil
	}

	out, err := exec.Command("glab", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("glab auth token failed (is glab installed and authenticated?): %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("glab auth token returned empty — run 'glab auth login' first")
	}
	return token, nil
}
