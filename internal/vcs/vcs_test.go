package vcs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeIdentityFieldsAreIndependent(t *testing.T) {
	wt := Worktree{
		RepositoryID: "repo-id",
		WorkspaceID:  "workspace-id",
		Name:         "repo/feature",
		NativeName:   "feature",
		Branch:       "refs/heads/feature",
		Path:         "/tmp/moved",
		Head:         "abc",
		ChangeID:     "change",
		VCS:          KindGit,
	}

	data, err := json.Marshal(wt)
	require.NoError(t, err)
	var decoded Worktree
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, wt, decoded)
	assert.NotEqual(t, decoded.Name, decoded.Branch)
	assert.NotEqual(t, decoded.WorkspaceID, decoded.Path)
}

func TestKindVocabulary(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		wantLabel   string
		wantNoun    string
		wantRefNoun string
	}{
		{
			name: "git", kind: KindGit,
			wantLabel: "git", wantNoun: "worktree", wantRefNoun: "branch",
		},
		{
			name: "jj", kind: KindJJ,
			wantLabel: "jj", wantNoun: "workspace", wantRefNoun: "workspace",
		},
		{
			// An unknown kind must degrade to git's vocabulary rather than print
			// empty strings into user-facing output.
			name: "unknown falls back to git wording", kind: Kind("hg"),
			wantLabel: "hg", wantNoun: "worktree", wantRefNoun: "branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantLabel, tt.kind.Label())
			assert.Equal(t, tt.wantNoun, tt.kind.Noun())
			assert.Equal(t, tt.wantRefNoun, tt.kind.RefNoun())
		})
	}
}

func TestParseKind(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Kind
		wantErr bool
	}{
		{name: "git", input: "git", want: KindGit},
		{name: "jj", input: "jj", want: KindJJ},
		{name: "empty", input: "", wantErr: true},
		{name: "unknown vcs", input: "hg", wantErr: true},
		{name: "case sensitive", input: "JJ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKind(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUnknownKind)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorktreePath(t *testing.T) {
	tests := []struct {
		name     string
		mainPath string
		ref      string
		want     string
	}{
		{
			name: "simple ref", mainPath: "/code/myrepo", ref: "feature",
			want: "/code/feature--myrepo",
		},
		{
			// Slashes become dashes so the checkout stays a single sibling dir.
			name: "slashes are flattened", mainPath: "/code/myrepo", ref: "feature/auth",
			want: "/code/feature-auth--myrepo",
		},
		{
			name: "nested slashes", mainPath: "/code/myrepo", ref: "a/b/c",
			want: "/code/a-b-c--myrepo",
		},
		{
			name: "pr ref", mainPath: "/code/myrepo", ref: "pr-711",
			want: "/code/pr-711--myrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, filepath.FromSlash(tt.want), WorktreePath(tt.mainPath, tt.ref))
		})
	}
}

func TestIsInside(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		root string
		want bool
	}{
		{name: "identical", cwd: "/a/b", root: "/a/b", want: true},
		{name: "direct child", cwd: "/a/b/c", root: "/a/b", want: true},
		{name: "deep child", cwd: "/a/b/c/d/e", root: "/a/b", want: true},
		{name: "trailing slash is normalised", cwd: "/a/b/", root: "/a/b", want: true},
		{name: "dot segments are normalised", cwd: "/a/b/c/..", root: "/a/b", want: true},
		{name: "sibling", cwd: "/a/c", root: "/a/b", want: false},
		{name: "parent is not inside child", cwd: "/a", root: "/a/b", want: false},
		{
			// A shared name prefix must not count as containment.
			name: "prefix but not a path boundary", cwd: "/a/bcd", root: "/a/b", want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsInside(tt.cwd, tt.root))
		})
	}
}

func TestSanitizedEnv(t *testing.T) {
	// git sets these for every hook it runs. Inheriting them would point git at
	// the wrong repository even though wtf named the one it meant.
	t.Setenv("GIT_DIR", ".git")
	t.Setenv("GIT_INDEX_FILE", ".git/index")
	t.Setenv("GIT_WORK_TREE", "/somewhere/else")
	t.Setenv("GIT_PREFIX", "sub/")
	// These must survive, or credentials and transports break.
	t.Setenv("GIT_SSH_COMMAND", "ssh -i /key")
	t.Setenv("PATH", os.Getenv("PATH"))

	env := SanitizedEnv()

	got := map[string]string{}
	for _, kv := range env {
		if name, val, ok := strings.Cut(kv, "="); ok {
			got[name] = val
		}
	}

	for _, dropped := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE", "GIT_PREFIX"} {
		assert.NotContains(t, got, dropped)
	}
	assert.Equal(t, "ssh -i /key", got["GIT_SSH_COMMAND"])
	assert.NotEmpty(t, got["PATH"])
}
