package git

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommitURL(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		hash      string
		want      string
	}{
		{
			name:      "github SSH",
			remoteURL: "git@github.com:AndrewPBerg/wtf.git",
			hash:      "a3bb6c0abc123",
			want:      "https://github.com/AndrewPBerg/wtf/commit/a3bb6c0abc123",
		},
		{
			name:      "github HTTPS",
			remoteURL: "https://github.com/AndrewPBerg/wtf.git",
			hash:      "a3bb6c0abc123",
			want:      "https://github.com/AndrewPBerg/wtf/commit/a3bb6c0abc123",
		},
		{
			name:      "github HTTPS no .git suffix",
			remoteURL: "https://github.com/AndrewPBerg/wtf",
			hash:      "abc1234",
			want:      "https://github.com/AndrewPBerg/wtf/commit/abc1234",
		},
		{
			name:      "gitlab SSH",
			remoteURL: "git@gitlab.com:group/project.git",
			hash:      "deadbeef",
			want:      "https://gitlab.com/group/project/commit/deadbeef",
		},
		{
			name:      "gitlab HTTPS",
			remoteURL: "https://gitlab.com/group/project.git",
			hash:      "deadbeef",
			want:      "https://gitlab.com/group/project/commit/deadbeef",
		},
		{
			name:      "ssh:// format",
			remoteURL: "ssh://git@github.com/user/repo.git",
			hash:      "abc1234",
			want:      "https://github.com/user/repo/commit/abc1234",
		},
		{
			name:      "http format",
			remoteURL: "http://github.com/user/repo.git",
			hash:      "abc1234",
			want:      "http://github.com/user/repo/commit/abc1234",
		},
		{
			name:      "empty remote",
			remoteURL: "",
			hash:      "abc1234",
			want:      "",
		},
		{
			name:      "unrecognized format",
			remoteURL: "ftp://something/weird",
			hash:      "abc1234",
			want:      "",
		},
		{
			name:      "remote with trailing whitespace",
			remoteURL: "git@github.com:user/repo.git  \n",
			hash:      "abc1234",
			want:      "https://github.com/user/repo/commit/abc1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CommitURL(tt.remoteURL, tt.hash)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRemoteURL(t *testing.T) {
	mock := newMockExecutor()
	mock.on("remote get-url origin", "git@github.com:user/repo.git", nil)
	wm := NewWorktreeManager(mock)

	url, err := wm.RemoteURL("/some/dir")
	assert.NoError(t, err)
	assert.Equal(t, "git@github.com:user/repo.git", url)
}

func TestRemoteURLError(t *testing.T) {
	mock := newMockExecutor()
	mock.on("remote get-url origin", "", fmt.Errorf("no remote"))
	wm := NewWorktreeManager(mock)

	url, err := wm.RemoteURL("/some/dir")
	assert.Error(t, err)
	assert.Empty(t, url)
}
