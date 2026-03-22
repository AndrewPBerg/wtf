package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// gitHub implements Forge for GitHub repositories.
type gitHub struct {
	host   string // e.g. "github.com"
	owner  string
	repo   string
	token  string
	apiURL string // e.g. "https://api.github.com" or GHE endpoint
	client *http.Client
}

func newGitHub(host, owner, repo string, tokenFn TokenFunc) (*gitHub, error) {
	token, err := tokenFn()
	if err != nil {
		return nil, fmt.Errorf("getting github token: %w", err)
	}

	apiURL := "https://api.github.com"
	if host != "github.com" {
		apiURL = "https://" + host + "/api/v3"
	}

	return &gitHub{
		host:   host,
		owner:  owner,
		repo:   repo,
		token:  token,
		apiURL: apiURL,
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (g *gitHub) Name() string { return "github" }

func (g *gitHub) PRURL(number int) string {
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", g.host, g.owner, g.repo, number)
}

func (g *gitHub) FetchRef(number int) string {
	return fmt.Sprintf("pull/%d/head:pr-%d", number, number)
}

// ghPR is the JSON shape returned by the GitHub REST API.
type ghPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Draft     bool   `json:"draft"`
	State     string `json:"state"`
	MergedAt  string `json:"merged_at"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (g *gitHub) ListPRs(ctx context.Context) ([]PR, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", g.apiURL, g.owner, g.repo)

	body, err := g.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("listing PRs: %w", err)
	}

	var ghPRs []ghPR
	if err := json.Unmarshal(body, &ghPRs); err != nil {
		return nil, fmt.Errorf("parsing PR list: %w", err)
	}

	prs := make([]PR, len(ghPRs))
	for i, p := range ghPRs {
		prs[i] = g.toPR(p)
	}
	return prs, nil
}

func (g *gitHub) GetPR(ctx context.Context, number int) (*PR, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", g.apiURL, g.owner, g.repo, number)

	body, err := g.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("getting PR #%d: %w", number, err)
	}

	var p ghPR
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parsing PR #%d: %w", number, err)
	}

	pr := g.toPR(p)
	return &pr, nil
}

func (g *gitHub) toPR(p ghPR) PR {
	created, _ := time.Parse(time.RFC3339, p.CreatedAt)
	return PR{
		Number:    p.Number,
		Title:     p.Title,
		Branch:    p.Head.Ref,
		Author:    p.User.Login,
		CreatedAt: created,
		URL:       p.HTMLURL,
		IsDraft:   p.Draft,
		State:     ghState(p),
	}
}

// ghState maps GitHub API state + merged_at to PRState.
func ghState(p ghPR) PRState {
	if p.MergedAt != "" {
		return PRMerged
	}
	if p.State == "closed" {
		return PRClosed
	}
	return PROpen
}

func (g *gitHub) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("API returned 401 Unauthorized — check your token or run 'gh auth login' to re-authenticate")
		case http.StatusForbidden:
			return nil, fmt.Errorf("API returned 403 Forbidden — your token may lack repo scope, or try 'gh auth login' to switch accounts")
		case http.StatusNotFound:
			if g.token == "" {
				return nil, fmt.Errorf("API returned 404 Not Found — no token provided, run 'gh auth login'")
			}
			return nil, fmt.Errorf("API returned 404 Not Found — repo may be private, check you're authenticated as the right user with 'gh auth status'")
		default:
			return nil, fmt.Errorf("API returned %s", resp.Status)
		}
	}

	var buf []byte
	buf, err = readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return buf, nil
}

// ghToken retrieves a GitHub token via the gh CLI.
func ghToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed (is gh installed and authenticated?): %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty — run 'gh auth login' first")
	}
	return token, nil
}
