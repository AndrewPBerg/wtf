package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WTF_HOME", home)
	return home
}

func TestDefaultWTFHome_EnvOverride(t *testing.T) {
	t.Setenv("WTF_HOME", "/tmp/test-wtf")
	assert.Equal(t, "/tmp/test-wtf", DefaultWTFHome())
}

func TestDefaultWTFHome_Default(t *testing.T) {
	t.Setenv("WTF_HOME", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".wtf"), DefaultWTFHome())
}

func TestRegistryPath(t *testing.T) {
	t.Setenv("WTF_HOME", "/tmp/test-wtf")
	assert.Equal(t, "/tmp/test-wtf/repos.json", RegistryPath())
}

func TestLoad_NoFile(t *testing.T) {
	setupTestHome(t)

	paths, err := Load()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestLoad_ValidFile(t *testing.T) {
	home := setupTestHome(t)

	data := `["/repo/a", "/repo/b"]`
	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte(data), 0o644))

	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/a", "/repo/b"}, paths)
}

func TestLoad_InvalidJSON(t *testing.T) {
	home := setupTestHome(t)

	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("not json"), 0o644))

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing registry")
}

func TestSave(t *testing.T) {
	home := setupTestHome(t)

	err := Save([]string{"/repo/x", "/repo/y"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(home, "repos.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "/repo/x")
	assert.Contains(t, string(data), "/repo/y")
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_HOME", filepath.Join(dir, "nested", "wtf"))

	err := Save([]string{"/repo/a"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "nested", "wtf", "repos.json"))
	require.NoError(t, err)
}

func TestAdd_NewRepo(t *testing.T) {
	setupTestHome(t)

	require.NoError(t, Add("/repo/a"))
	require.NoError(t, Add("/repo/b"))

	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/a", "/repo/b"}, paths)
}

func TestAdd_Duplicate(t *testing.T) {
	setupTestHome(t)

	require.NoError(t, Add("/repo/a"))
	require.NoError(t, Add("/repo/a"))

	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/a"}, paths)
}

func TestLoadValid_FiltersStale(t *testing.T) {
	home := setupTestHome(t)

	repoDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	require.NoError(t, Save([]string{repoDir, "/nonexistent/repo"}))

	valid, err := LoadValid()
	require.NoError(t, err)
	assert.Equal(t, []string{repoDir}, valid)

	// Verify the registry was NOT modified (still has stale entry)
	paths, err := Load()
	require.NoError(t, err)
	assert.Len(t, paths, 2, "LoadValid should not modify the registry file")
	_ = home
}

func TestLoadValid_LoadError(t *testing.T) {
	home := setupTestHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("bad"), 0o644))

	_, err := LoadValid()
	assert.Error(t, err)
}

func TestPrune_RemovesStale(t *testing.T) {
	setupTestHome(t)

	// Create a real git repo
	repoDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	require.NoError(t, Save([]string{repoDir, "/nonexistent/repo"}))

	valid, err := Prune()
	require.NoError(t, err)
	assert.Equal(t, []string{repoDir}, valid)

	// Verify it was persisted
	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{repoDir}, paths)
}

func TestPrune_EmptyRegistry(t *testing.T) {
	setupTestHome(t)

	valid, err := Prune()
	require.NoError(t, err)
	assert.Empty(t, valid)
}

func TestPrune_AllStale(t *testing.T) {
	setupTestHome(t)

	require.NoError(t, Save([]string{"/gone/a", "/gone/b"}))

	valid, err := Prune()
	require.NoError(t, err)
	assert.Empty(t, valid)
}

func TestLoad_UnreadableFile(t *testing.T) {
	home := setupTestHome(t)

	// Create a directory where the file should be — os.ReadFile will fail
	require.NoError(t, os.MkdirAll(filepath.Join(home, "repos.json"), 0o755))

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading registry")
}

func TestSave_UnwritableDir(t *testing.T) {
	// Point to a path under /dev/null which can't be a directory
	t.Setenv("WTF_HOME", "/dev/null/impossible")

	err := Save([]string{"/repo/a"})
	assert.Error(t, err)
}

func TestAdd_LoadError(t *testing.T) {
	home := setupTestHome(t)

	// Corrupt the registry file
	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("bad"), 0o644))

	err := Add("/repo/a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing registry")
}

func TestPrune_LoadError(t *testing.T) {
	home := setupTestHome(t)

	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("bad"), 0o644))

	_, err := Prune()
	assert.Error(t, err)
}

func TestSave_EmptySlice(t *testing.T) {
	setupTestHome(t)

	err := Save([]string{})
	require.NoError(t, err)

	paths, err := Load()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestRemove_Existing(t *testing.T) {
	setupTestHome(t)
	require.NoError(t, Add("/repo/a"))
	require.NoError(t, Add("/repo/b"))

	removed, err := Remove("/repo/a")
	require.NoError(t, err)
	assert.True(t, removed)

	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/b"}, paths)
}

func TestRemove_NotFound(t *testing.T) {
	setupTestHome(t)
	require.NoError(t, Add("/repo/a"))

	removed, err := Remove("/repo/z")
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestRemove_EmptyRegistry(t *testing.T) {
	setupTestHome(t)

	removed, err := Remove("/repo/x")
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestRemove_LoadError(t *testing.T) {
	home := setupTestHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "repos.json"), []byte("bad"), 0o644))

	_, err := Remove("/repo/a")
	assert.Error(t, err)
}

func TestIsRepo(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  bool
	}{
		{
			name: "valid git repo",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
				return dir
			},
			want: true,
		},
		{
			name: "jj editor Git shadow is not a primary repo",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", vcs.JJGitDiffMarker), []byte("shadow\n"), 0o644))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".jj"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".jj", "repo"), []byte("../../main/.jj/repo\n"), 0o644))
				return dir
			},
			want: false,
		},
		{
			name: "no .git dir",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			want: false,
		},
		{
			name: "nonexistent path",
			setup: func(_ *testing.T) string {
				return "/nonexistent/path"
			},
			want: false,
		},
		{
			name: ".git is a file not dir",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ..."), 0o644))
				return dir
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			assert.Equal(t, tt.want, IsRepo(path))
		})
	}
}

func TestReadLegacyRegistryFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_HOME", dir)

	// v1 on-disk format: a bare array of paths.
	require.NoError(t, os.WriteFile(RegistryPath(),
		[]byte(`["/code/a", "/code/b"]`), 0o644))

	paths, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"/code/a", "/code/b"}, paths)
}

func TestLegacyRegistryUpgradesOnSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_HOME", dir)

	require.NoError(t, os.WriteFile(RegistryPath(), []byte(`["/code/a"]`), 0o644))
	require.NoError(t, Add("/code/b"))

	data, err := os.ReadFile(RegistryPath())
	require.NoError(t, err)

	var reg struct {
		Version int `json:"version"`
		Repos   []struct {
			Path string `json:"path"`
		} `json:"repos"`
	}
	require.NoError(t, json.Unmarshal(data, &reg))
	assert.Equal(t, 2, reg.Version)
	require.Len(t, reg.Repos, 2)
	assert.Equal(t, "/code/a", reg.Repos[0].Path)
	assert.Equal(t, "/code/b", reg.Repos[1].Path)
}

func TestVCSPrefRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_HOME", dir)

	_, ok := VCSPref("/code/a")
	assert.False(t, ok, "no preference recorded yet")

	require.NoError(t, SetVCSPref("/code/a", vcs.KindJJ))

	got, ok := VCSPref("/code/a")
	require.True(t, ok)
	assert.Equal(t, vcs.KindJJ, got)

	// SetVCSPref registers a previously unknown repo.
	paths, err := Load()
	require.NoError(t, err)
	assert.Contains(t, paths, "/code/a")

	// Overwrite.
	require.NoError(t, SetVCSPref("/code/a", vcs.KindGit))
	got, ok = VCSPref("/code/a")
	require.True(t, ok)
	assert.Equal(t, vcs.KindGit, got)

	require.NoError(t, ClearVCSPref("/code/a"))
	_, ok = VCSPref("/code/a")
	assert.False(t, ok)
}

func TestSavePreservesVCSPrefsAndDropsRemoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WTF_HOME", dir)

	require.NoError(t, SetVCSPref("/code/keep", vcs.KindJJ))
	require.NoError(t, SetVCSPref("/code/drop", vcs.KindGit))

	// Saving a narrower list must keep the surviving repo's preference and
	// discard the removed one's.
	require.NoError(t, Save([]string{"/code/keep"}))

	got, ok := VCSPref("/code/keep")
	require.True(t, ok)
	assert.Equal(t, vcs.KindJJ, got)

	_, ok = VCSPref("/code/drop")
	assert.False(t, ok)
}

func TestIsRepoAcceptsJJRepo(t *testing.T) {
	// A primary jj workspace: .jj/repo is a directory.
	main := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(main, ".jj", "repo"), 0o755))
	assert.True(t, IsRepo(main))

	// A secondary jj workspace: .jj/repo is a file pointer, so it must be
	// rejected just as a git worktree's .git file is.
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".jj"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".jj", "repo"),
		[]byte("../../main/.jj/repo"), 0o644))
	assert.False(t, IsRepo(ws))
}
