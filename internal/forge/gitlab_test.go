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

func TestGitLabListPRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/projects/org/project/merge_requests")
		assert.Equal(t, "opened", r.URL.Query().Get("state"))
		assert.Equal(t, "gl-token", r.Header.Get("PRIVATE-TOKEN"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"iid": 10,
				"title": "Add widget",
				"web_url": "https://gitlab.com/org/project/-/merge_requests/10",
				"created_at": "2025-02-01T09:00:00Z",
				"draft": false,
				"source_branch": "widget-branch",
				"author": {"username": "dave"}
			}
		]`))
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "gl-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	prs, err := gl.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)

	assert.Equal(t, 10, prs[0].Number)
	assert.Equal(t, "Add widget", prs[0].Title)
	assert.Equal(t, "widget-branch", prs[0].Branch)
	assert.Equal(t, "dave", prs[0].Author)
}

func TestGitLabGetPR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/projects/org/project/merge_requests/10")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"iid": 10,
			"title": "Add widget",
			"web_url": "https://gitlab.com/org/project/-/merge_requests/10",
			"created_at": "2025-02-01T09:00:00Z",
			"draft": true,
			"source_branch": "widget-branch",
			"author": {"username": "dave"}
		}`))
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "gl-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	pr, err := gl.GetPR(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 10, pr.Number)
	assert.Equal(t, "Add widget", pr.Title)
	assert.True(t, pr.IsDraft)
}

func TestGitLabAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "bad-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	_, err := gl.ListPRs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestGitLabGetPR_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "gl-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	_, err := gl.GetPR(context.Background(), 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGitLabListPRs_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "gl-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	_, err := gl.ListPRs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing MR list")
}

func TestGitLabGetPR_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	gl := &gitLab{
		host:      "gitlab.com",
		owner:     "org",
		repo:      "project",
		token:     "gl-token",
		apiURL:    srv.URL,
		projectID: "org%2Fproject",
		client:    srv.Client(),
	}

	_, err := gl.GetPR(context.Background(), 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing MR !10")
}

func TestNewGitLab_TokenError(t *testing.T) {
	failToken := func() (string, error) { return "", fmt.Errorf("no token") }
	_, err := newGitLab("gitlab.com", "org", "project", failToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting gitlab token")
}
