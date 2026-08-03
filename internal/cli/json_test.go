package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	buf := new(bytes.Buffer)
	err := writeJSON(buf, map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"key": "value"`)
}

func TestIsJSONOutput(t *testing.T) {
	saved := jsonOutput
	defer func() { jsonOutput = saved }()

	jsonOutput = false
	assert.False(t, IsJSONOutput())

	jsonOutput = true
	assert.True(t, IsJSONOutput())
}

func TestVersionCommand_JSON(t *testing.T) {
	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = true

	buf := new(bytes.Buffer)
	cmd := versionCmd
	cmd.SetOut(buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, Version, result["version"])
}

func TestSwCommand_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)
	wm := git.NewWorktreeManager(&git.RealExecutor{})

	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = true

	_, err := wm.Add(dir, "feat-json", "main")
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(buf)

	err = runSw(cmd, "feat-json", wm)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "feat-json", result["branch"])
	assert.Contains(t, result["path"], "feat-json")
}

func TestNewCommand_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = true

	buf := new(bytes.Buffer)
	cmd := newCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	newBase = "main"
	err := runNew(cmd, "feat-new-json", newBase, wm, nil, false)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "feat-new-json", result["branch"])
	assert.Contains(t, result["path"], "feat-new-json")
}

func TestNewsCommand_JSON(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = true

	buf := new(bytes.Buffer)
	cmd := newsCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	err := runNews(cmd, "feat-news-json", wm, nil)
	require.NoError(t, err)

	var result map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "feat-news-json", result["branch"])
	assert.Contains(t, result["path"], "feat-news-json")
}

func TestCleanCommand_JSON_NothingToClean(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	saved := jsonOutput
	defer func() { jsonOutput = saved }()
	jsonOutput = true

	buf := new(bytes.Buffer)
	cmd := cleanCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	exec := &git.RealExecutor{}
	err := runClean(cmd, wm, exec)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, false, result["dry_run"])
	removed, ok := result["removed"].([]any)
	require.True(t, ok)
	assert.Empty(t, removed)
}
