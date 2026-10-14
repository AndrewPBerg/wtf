package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
)

// This is the discovery phase of scripts/github-graphql-spike.py. Cleanup
// needs exact merged heads, not the spike's second phase of CI hydration.
type removalGHClient struct {
	binary  string
	targets map[string][]prSnapshot
}

type removalTarget struct {
	kind, value, cursor string
	pages               int
}

type removalConnection struct {
	Nodes    []PullRequest `json:"nodes"`
	PageInfo *struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

func removalTargets(snapshots []prSnapshot) []removalTarget {
	seen := map[string]removalTarget{}
	for _, snapshot := range snapshots {
		revisions := append([]prRevision{{Commit: snapshot.Commit, Bookmarks: snapshot.Bookmarks}}, snapshot.Parents...)
		for _, revision := range revisions {
			if revision.Commit != "" {
				seen["commit:"+revision.Commit] = removalTarget{kind: "commit", value: revision.Commit}
			}
			for _, branch := range revision.Bookmarks {
				seen["branch:"+branch] = removalTarget{kind: "branch", value: branch}
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	targets := make([]removalTarget, 0, len(keys))
	for _, key := range keys {
		targets = append(targets, seen[key])
	}
	return targets
}

func graphqlString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func removalQuery(repository string, targets []removalTarget) string {
	owner, name, _ := strings.Cut(repository, "/")
	const fields = `number url state isDraft headRefName headRefOid baseRefName isCrossRepository headRepositoryOwner { login }`
	var query strings.Builder
	query.WriteString("query {")
	for i, target := range targets {
		connection := `first:100`
		if target.cursor != "" {
			connection += ",after:" + graphqlString(target.cursor)
		}
		selection := "{ pageInfo { hasNextPage endCursor } nodes { " + fields + " } }"
		field := "pullRequests(headRefName:" + graphqlString(target.value) + "," + connection + ") " + selection
		if target.kind == "commit" {
			field = "object(oid:" + graphqlString(target.value) + ") { ... on Commit { associatedPullRequests(" + connection + ") " + selection + " } }"
		}
		fmt.Fprintf(&query, " r%d:repository(owner:%s,name:%s) { %s }", i, graphqlString(owner), graphqlString(name), field)
	}
	query.WriteString(" }")
	return query.String()
}

func (client removalGHClient) List(ctx context.Context, repository string) ([]PullRequest, error) {
	if !githubSlug.MatchString(repository) {
		return nil, Fail("invalid_github_repository", "Invalid GitHub repository")
	}
	pending := removalTargets(client.targets[repository])
	prs := map[int]PullRequest{}
	binary := (GHClient{Binary: client.binary}).binary(ctx)
	for len(pending) > 0 {
		batch := pending[:min(50, len(pending))]
		pending = pending[len(batch):]
		payload, err := json.Marshal(map[string]string{"query": removalQuery(repository, batch)})
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, binary, "api", "graphql", "--input", "-")
		cmd.Stdin = bytes.NewReader(payload)
		var data bytes.Buffer
		cmd.Stdout, cmd.Stderr = &data, io.Discard
		configureCommand(cmd)
		err = cmd.Run()
		finishCommand(cmd)
		if err != nil {
			return nil, Fail("github_unavailable", "GitHub lookup failed; check gh authentication or network access")
		}
		var response struct {
			Data map[string]*struct {
				PullRequests *removalConnection `json:"pullRequests"`
				Object       json.RawMessage    `json:"object"`
			} `json:"data"`
			Errors []json.RawMessage `json:"errors"`
		}
		if json.Unmarshal(data.Bytes(), &response) != nil || len(response.Errors) > 0 || response.Data == nil {
			return nil, Fail("invalid_github_response", "GitHub returned an incomplete GraphQL response")
		}
		for i, target := range batch {
			result := response.Data[fmt.Sprintf("r%d", i)]
			if result == nil {
				return nil, Fail("github_unavailable", "GitHub repository is unavailable")
			}
			connection := result.PullRequests
			if target.kind == "commit" {
				// Local-only commits legitimately have no object on GitHub.
				if bytes.Equal(bytes.TrimSpace(result.Object), []byte("null")) {
					continue
				}
				var object struct {
					Associated *removalConnection `json:"associatedPullRequests"`
				}
				if json.Unmarshal(result.Object, &object) != nil {
					return nil, Fail("invalid_github_response", "GitHub returned an invalid commit observation")
				}
				connection = object.Associated
			}
			if connection == nil || connection.PageInfo == nil || connection.Nodes == nil {
				return nil, Fail("invalid_github_response", "GitHub returned an incomplete PR connection")
			}
			for _, pr := range connection.Nodes {
				if pr.Number <= 0 || pr.Head == "" || pr.State == "" || pr.Base == "" {
					return nil, Fail("invalid_github_response", "GitHub returned incomplete PR metadata")
				}
				prs[pr.Number] = pr
			}
			if connection.PageInfo.HasNextPage {
				if target.pages >= 1 || connection.PageInfo.EndCursor == "" || connection.PageInfo.EndCursor == target.cursor {
					return nil, Fail("github_lookup_incomplete", "GitHub PR discovery exceeded its bounded pages")
				}
				target.cursor = connection.PageInfo.EndCursor
				target.pages++
				pending = append(pending, target)
			}
		}
	}
	out := make([]PullRequest, 0, len(prs))
	for _, pr := range prs {
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}
