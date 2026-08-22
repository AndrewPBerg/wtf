package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceListReportHasStableEnvelope(t *testing.T) {
	data, err := json.Marshal(workspaceListReport{Version: 1, Workspaces: []workspaceReport{}})
	assert.NoError(t, err)
	assert.Equal(t, `{"version":1,"workspaces":[]}`, string(data))
}

func TestInspectGitDiffShadowStatuses(t *testing.T) {
	absent := inspectGitDiffShadow(t.TempDir(), "")
	assert.Equal(t, "absent", absent.Status)

	repo := initCLITestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", vcs.JJGitDiffMarker), []byte("shadow\n"), 0o644))
	head, err := (&git.RealExecutor{}).Run(repo, "rev-parse", "--short=12", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "present", inspectGitDiffShadow(repo, head).Status)
	assert.Equal(t, "stale", inspectGitDiffShadow(repo, "different").Status)

	broken := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(broken, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(broken, ".git", vcs.JJGitDiffMarker), []byte("shadow\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(broken, ".git", "HEAD"), []byte("broken\n"), 0o644))
	unavailable := inspectGitDiffShadow(broken, "anything")
	assert.Equal(t, "unavailable", unavailable.Status)
	assert.NotEmpty(t, unavailable.Error)
}

func TestPrintWorkspaceReportSeparatesPhysicalState(t *testing.T) {
	cmd := workspaceInspectCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	printWorkspaceReport(cmd, workspaceReport{
		Version:       1,
		Identity:      identity.Workspace{ID: "id", Name: "repo/main", Path: "/tmp/repo", Backend: "jj", NativeName: "repo/main", LifecycleState: identity.Removed},
		Physical:      physicalReport{Path: "/tmp/repo"},
		GitDiffShadow: shadowReport{Supported: true, Status: "absent"},
	})
	assert.Contains(t, buf.String(), "removed (missing)")
	assert.Contains(t, buf.String(), "git-diff-shadow: absent")
}
