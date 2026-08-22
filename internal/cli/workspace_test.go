package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/stretchr/testify/assert"
)

func TestWorkspaceListReportHasStableEnvelope(t *testing.T) {
	data, err := json.Marshal(workspaceListReport{Workspaces: []workspaceReport{}})
	assert.NoError(t, err)
	assert.Equal(t, `{"workspaces":[]}`, string(data))
}

func TestPrintWorkspaceReportSeparatesPhysicalState(t *testing.T) {
	cmd := workspaceInspectCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	printWorkspaceReport(cmd, workspaceReport{
		Identity:      identity.Workspace{ID: "id", Name: "repo/main", Path: "/tmp/repo", Backend: "jj", NativeName: "repo/main", LifecycleState: identity.Removed},
		Physical:      physicalReport{Path: "/tmp/repo"},
		GitDiffShadow: shadowReport{Supported: true, Status: "absent"},
	})
	assert.Contains(t, buf.String(), "removed (missing)")
	assert.Contains(t, buf.String(), "git-diff-shadow: absent")
}
