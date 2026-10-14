package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func removalGraphQLFixture(t *testing.T, response string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\ncat > \"$WTF_GRAPHQL_TEST_INPUT\"\ncat <<'RESPONSE'\n" + response + "\nRESPONSE\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WTF_GRAPHQL_TEST_INPUT", filepath.Join(t.TempDir(), "query.json"))
	return path
}

func TestRemovalGraphQLValidatesPartialAndMissingResponses(t *testing.T) {
	for _, test := range []struct {
		name, response, code string
		count                int
	}{
		{"merged", `{"data":{"r0":{"object":{"associatedPullRequests":{"nodes":[{"number":42,"headRefOid":"published","baseRefName":"main","state":"MERGED"}],"pageInfo":{"hasNextPage":false}}}}}}`, "", 1},
		{"local-only", `{"data":{"r0":{"object":null}}}`, "", 0},
		{"empty", `{"data":{"r0":{"object":{"associatedPullRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`, "", 0},
		{"partial", `{"data":{"r0":{"object":null}},"errors":[{"message":"secret provider diagnostics"}]}`, "invalid_github_response", 0},
		{"missing-repository", `{"data":{"r0":null}}`, "github_unavailable", 0},
		{"missing-alias", `{"data":{}}`, "github_unavailable", 0},
		{"missing-object", `{"data":{"r0":{}}}`, "invalid_github_response", 0},
		{"missing-page-info", `{"data":{"r0":{"object":{"associatedPullRequests":{"nodes":[]}}}}}`, "invalid_github_response", 0},
		{"missing-nodes", `{"data":{"r0":{"object":{"associatedPullRequests":{"pageInfo":{"hasNextPage":false}}}}}}`, "invalid_github_response", 0},
		{"incomplete-pr", `{"data":{"r0":{"object":{"associatedPullRequests":{"nodes":[{"number":42}],"pageInfo":{"hasNextPage":false}}}}}}`, "invalid_github_response", 0},
		{"missing-cursor", `{"data":{"r0":{"object":{"associatedPullRequests":{"nodes":[],"pageInfo":{"hasNextPage":true}}}}}}`, "github_lookup_incomplete", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := removalGHClient{binary: removalGraphQLFixture(t, test.response), targets: map[string][]prSnapshot{"owner/repo": {{Commit: "published"}}}}
			prs, err := client.List(t.Context(), "owner/repo")
			if (test.code == "" && err != nil) || (test.code != "" && ErrorCode(err) != test.code) || len(prs) != test.count {
				t.Fatalf("got %d PRs, %v; wanted %d, %s", len(prs), err, test.count, test.code)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("provider diagnostics exposed")
			}
			payload, err := os.ReadFile(os.Getenv("WTF_GRAPHQL_TEST_INPUT"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(payload), "statusCheckRollup") {
				t.Fatal("cleanup requested CI hydration")
			}
		})
	}
}

func TestRemovalGraphQLPaginatesAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_GRAPHQL_TEST_INPUT", filepath.Join(dir, "query.json"))
	t.Setenv("WTF_GRAPHQL_TEST_MARKER", filepath.Join(dir, "once"))
	path := filepath.Join(dir, "gh")
	const pr = `{"number":42,"headRefOid":"published","baseRefName":"main","state":"MERGED"}`
	script := `#!/bin/sh
cat > "$WTF_GRAPHQL_TEST_INPUT"
if test -f "$WTF_GRAPHQL_TEST_MARKER"; then
cat <<'RESPONSE'
{"data":{"r0":{"pullRequests":{"nodes":[` + pr + `],"pageInfo":{"hasNextPage":false}}}}}
RESPONSE
else
touch "$WTF_GRAPHQL_TEST_MARKER"
cat <<'RESPONSE'
{"data":{"r0":{"pullRequests":{"nodes":[` + pr + `],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}
RESPONSE
fi
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	client := removalGHClient{binary: path, targets: map[string][]prSnapshot{"owner/repo": {{Bookmarks: []string{"topic"}}}}}
	prs, err := client.List(t.Context(), "owner/repo")
	if err != nil || len(prs) != 1 {
		t.Fatalf("pagination: %+v %v", prs, err)
	}
	payload, err := os.ReadFile(os.Getenv("WTF_GRAPHQL_TEST_INPUT"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct{ Query string }
	if err = json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.Query, `after:"next"`) {
		t.Fatal("second page did not use cursor")
	}
}

func TestRemovalGraphQLTransientResultsDoNotReplaceBrowserCache(t *testing.T) {
	home := t.TempDir()
	old := prCacheEntry{Repository: "owner/repo", CheckedAt: time.Now(), PRs: []PullRequest{pr(99, "topic", "old")}}
	service := PRService{Home: home, Client: fakePRClient{call: func(context.Context, string) ([]PullRequest, error) {
		return []PullRequest{pr(42, "topic", "new")}, nil
	}}, Now: time.Now}
	if err := service.save(t.Context(), map[string]prCacheEntry{"repo": old}); err != nil {
		t.Fatal(err)
	}
	plan := PRPlan{Groups: []prGroup{{ID: "repo", Repository: "owner/repo", Refresh: true, Transient: true, Snapshot: map[string]prSnapshot{"work": {Commit: "new"}}}}, Rows: []WorkspaceView{{Workspace: Workspace{Name: "work"}, RepositoryID: "repo", Exists: true}}}
	rows := service.Refresh(t.Context(), plan)
	if len(rows[0].GitHub.Matches) != 1 || rows[0].GitHub.Matches[0].Number != 42 || rows[0].GitHub.Lookup != "fresh" {
		t.Fatal("targeted result not published")
	}
	if cached := service.cache().Repositories["repo"]; len(cached.PRs) != 1 || cached.PRs[0].Number != 99 {
		t.Fatal("targeted subset replaced browser cache")
	}
}

func TestRemovalGraphQLLive(t *testing.T) {
	root := os.Getenv("WTF_REMOVAL_GRAPHQL_LIVE_ROOT")
	if root == "" {
		t.Skip("set WTF_REMOVAL_GRAPHQL_LIVE_ROOT for live read-only validation")
	}
	snapshots, err := snapshots(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repoGitHub(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	var targets []prSnapshot
	for _, snapshot := range snapshots {
		targets = append(targets, snapshot)
	}
	client := removalGHClient{targets: map[string][]prSnapshot{repo: targets}}
	start := time.Now()
	prs, err := client.List(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live targeted discovery: %d workspaces, %d PRs, %s", len(targets), len(prs), time.Since(start))
}
