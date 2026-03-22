package forge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubListPRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/user/repo/pulls", r.URL.Path)
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"number": 1,
				"title": "Add feature",
				"draft": false,
				"html_url": "https://github.com/user/repo/pull/1",
				"created_at": "2025-01-15T10:00:00Z",
				"head": {"ref": "feature-branch"},
				"user": {"login": "alice"}
			},
			{
				"number": 2,
				"title": "Fix bug",
				"draft": true,
				"html_url": "https://github.com/user/repo/pull/2",
				"created_at": "2025-01-16T12:00:00Z",
				"head": {"ref": "fix-bug"},
				"user": {"login": "bob"}
			}
		]`))
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	prs, err := gh.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 2)

	assert.Equal(t, 1, prs[0].Number)
	assert.Equal(t, "Add feature", prs[0].Title)
	assert.Equal(t, "feature-branch", prs[0].Branch)
	assert.Equal(t, "alice", prs[0].Author)
	assert.False(t, prs[0].IsDraft)

	assert.Equal(t, 2, prs[1].Number)
	assert.Equal(t, "Fix bug", prs[1].Title)
	assert.Equal(t, "bob", prs[1].Author)
	assert.True(t, prs[1].IsDraft)
}

func TestGitHubGetPR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/user/repo/pulls/42", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 42,
			"title": "The answer",
			"draft": false,
			"html_url": "https://github.com/user/repo/pull/42",
			"created_at": "2025-03-01T08:00:00Z",
			"head": {"ref": "the-answer"},
			"user": {"login": "charlie"}
		}`))
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	pr, err := gh.GetPR(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "The answer", pr.Title)
	assert.Equal(t, "the-answer", pr.Branch)
	assert.Equal(t, "charlie", pr.Author)
}

func TestGitHubAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	_, err := gh.ListPRs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGitHubNoToken(t *testing.T) {
	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "",
		apiURL: "https://api.github.com",
		client: http.DefaultClient,
	}
	assert.Equal(t, "github", gh.Name())
}

func TestGitHubGetPR_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	_, err := gh.GetPR(context.Background(), 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGitHubListPRs_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	_, err := gh.ListPRs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing PR list")
}

func TestGitHubGetPR_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	gh := &gitHub{
		host:   "github.com",
		owner:  "user",
		repo:   "repo",
		token:  "test-token",
		apiURL: srv.URL,
		client: srv.Client(),
	}

	_, err := gh.GetPR(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing PR #1")
}

func TestGitHubEnterpriseAPIURL(t *testing.T) {
	fakeToken := func() (string, error) { return "test", nil }
	gh, err := newGitHub("github.example.com", "team", "app", fakeToken)
	require.NoError(t, err)
	assert.Equal(t, "https://github.example.com/api/v3", gh.apiURL)
}

func TestNewGitHub_TokenError(t *testing.T) {
	failToken := func() (string, error) { return "", fmt.Errorf("no token") }
	_, err := newGitHub("github.com", "user", "repo", failToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting github token")
}
