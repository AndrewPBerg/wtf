package forge

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		name          string
		remote        string
		wantHost      string
		wantOwner     string
		wantRepo      string
		wantEmptyHost bool
	}{
		{
			name:      "github ssh",
			remote:    "git@github.com:user/repo.git",
			wantHost:  "github.com",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "github https",
			remote:    "https://github.com/user/repo.git",
			wantHost:  "github.com",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "github https no .git",
			remote:    "https://github.com/user/repo",
			wantHost:  "github.com",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "github ssh://",
			remote:    "ssh://git@github.com/user/repo.git",
			wantHost:  "github.com",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:      "gitlab ssh",
			remote:    "git@gitlab.com:org/project.git",
			wantHost:  "gitlab.com",
			wantOwner: "org",
			wantRepo:  "project",
		},
		{
			name:      "gitlab https",
			remote:    "https://gitlab.com/org/project.git",
			wantHost:  "gitlab.com",
			wantOwner: "org",
			wantRepo:  "project",
		},
		{
			name:      "github enterprise ssh",
			remote:    "git@github.example.com:team/app.git",
			wantHost:  "github.example.com",
			wantOwner: "team",
			wantRepo:  "app",
		},
		{
			name:      "with trailing whitespace",
			remote:    "  git@github.com:user/repo.git  \n",
			wantHost:  "github.com",
			wantOwner: "user",
			wantRepo:  "repo",
		},
		{
			name:          "empty string",
			remote:        "",
			wantEmptyHost: true,
		},
		{
			name:          "unrecognized format",
			remote:        "ftp://example.com/repo",
			wantEmptyHost: true,
		},
		{
			name:          "ssh missing path",
			remote:        "git@github.com:",
			wantEmptyHost: true,
		},
		{
			name:          "missing repo",
			remote:        "git@github.com:user",
			wantEmptyHost: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, owner, repo := parseRemote(tt.remote)
			if tt.wantEmptyHost {
				assert.Empty(t, host)
				return
			}
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestDetect(t *testing.T) {
	fakeToken := func() (string, error) { return "test-token", nil }

	tests := []struct {
		name    string
		remote  string
		want    string
		wantErr bool
	}{
		{
			name:   "github detected",
			remote: "git@github.com:user/repo.git",
			want:   "github",
		},
		{
			name:   "gitlab detected",
			remote: "git@gitlab.com:org/project.git",
			want:   "gitlab",
		},
		{
			name:    "unsupported host",
			remote:  "git@bitbucket.org:user/repo.git",
			wantErr: true,
		},
		{
			name:    "unrecognized format",
			remote:  "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Detect(tt.remote, WithTokenFunc(fakeToken))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, f.Name())
		})
	}
}

func TestGitHubPRURL(t *testing.T) {
	gh := &gitHub{host: "github.com", owner: "user", repo: "app"}
	assert.Equal(t, "https://github.com/user/app/pull/42", gh.PRURL(42))
}

func TestGitHubFetchRef(t *testing.T) {
	gh := &gitHub{host: "github.com", owner: "user", repo: "app"}
	assert.Equal(t, "pull/42/head:pr-42", gh.FetchRef(42))
}

func TestGitLabPRURL(t *testing.T) {
	gl := &gitLab{host: "gitlab.com", owner: "org", repo: "project"}
	assert.Equal(t, "https://gitlab.com/org/project/-/merge_requests/10", gl.PRURL(10))
}

func TestGitLabFetchRef(t *testing.T) {
	gl := &gitLab{host: "gitlab.com", owner: "org", repo: "project"}
	assert.Equal(t, "merge-requests/10/head:mr-10", gl.FetchRef(10))
}

func TestGitHubEnterprise(t *testing.T) {
	fakeToken := func() (string, error) { return "test-token", nil }
	f, err := Detect("git@github.example.com:team/app.git", WithTokenFunc(fakeToken))
	require.NoError(t, err)
	assert.Equal(t, "github", f.Name())
	assert.Equal(t, "https://github.example.com/team/app/pull/5", f.PRURL(5))
}

func TestDetect_TokenError(t *testing.T) {
	failToken := func() (string, error) { return "", fmt.Errorf("no token") }
	_, err := Detect("git@github.com:user/repo.git", WithTokenFunc(failToken))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting github token")
}

func TestGitHubName(t *testing.T) {
	gh := &gitHub{}
	assert.Equal(t, "github", gh.Name())
}

func TestGitLabName(t *testing.T) {
	gl := &gitLab{}
	assert.Equal(t, "gitlab", gl.Name())
}
