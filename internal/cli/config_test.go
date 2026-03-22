package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInit_CreatesFile(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Created")
	assert.Contains(t, buf.String(), config.ProjectConfigFile)

	// Verify the file exists and has correct defaults
	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `default_base = "main"`)
	assert.Contains(t, content, `strategy = "symlink"`)
	assert.Contains(t, content, `".env", ".env.local"`)
}

func TestConfigInit_DetectsMainBranch(t *testing.T) {
	dir := t.TempDir()
	exec := &git.RealExecutor{}

	// Init with a non-standard default branch
	_, err := exec.Run(dir, "init", "-b", "develop")
	require.NoError(t, err)
	_, err = exec.Run(dir, "config", "user.email", "test@test.com")
	require.NoError(t, err)
	_, err = exec.Run(dir, "config", "user.name", "Test")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644))
	_, err = exec.Run(dir, "add", ".")
	require.NoError(t, err)
	_, err = exec.Run(dir, "commit", "-m", "initial")
	require.NoError(t, err)

	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(exec)
	bm := git.NewBranchManager(exec)
	err = runConfigInit(cmd, wm, bm)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	assert.Contains(t, string(data), `default_base = "develop"`)
}

func TestConfigInit_DetectsEnvFiles(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Create an .env file in the repo
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=x"), 0o644))

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `strategy = "symlink"`)
	assert.Contains(t, content, `".env"`)
}

func TestConfigInit_DetectsPackageManager(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Create a package-lock.json to simulate npm
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644))

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	assert.Contains(t, string(data), "npm install")
}

func TestConfigInit_AlreadyExists_Abort(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Pre-create the config file
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte("[worktree]\n"), 0o644))

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("n\n"))

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")

	// Original content should be preserved
	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	assert.Equal(t, "[worktree]\n", string(data))
}

func TestConfigInit_AlreadyExists_Overwrite(t *testing.T) {
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	// Pre-create the config file with old content
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ProjectConfigFile), []byte("[worktree]\n"), 0o644))

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("y\n"))

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created")

	// File should have new auto-detected content
	data, err := os.ReadFile(filepath.Join(dir, config.ProjectConfigFile))
	require.NoError(t, err)
	assert.Contains(t, string(data), `default_base = "main"`)
}

func TestConfigInit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	buf := new(bytes.Buffer)
	cmd := configInitCmd
	cmd.SetOut(buf)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	bm := git.NewBranchManager(&git.RealExecutor{})
	err := runConfigInit(cmd, wm, bm)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotARepo)
}
