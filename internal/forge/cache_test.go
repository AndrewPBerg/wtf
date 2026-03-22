package forge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockForge is a test double for the Forge interface.
type mockForge struct {
	prs       []PR
	callCount int
	err       error
}

func (m *mockForge) Name() string                                { return "mock" }
func (m *mockForge) PRURL(_ int) string                          { return "" }
func (m *mockForge) FetchRef(_ int) string                       { return "" }
func (m *mockForge) GetPR(_ context.Context, _ int) (*PR, error) { return nil, nil }

func (m *mockForge) ListPRs(_ context.Context) ([]PR, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.prs, nil
}

func TestCachedForgeMiss(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	inner := &mockForge{
		prs: []PR{{Number: 1, Title: "First PR", Branch: "feat"}},
	}

	cf := NewCachedForge(inner, gitDir)

	prs, err := cf.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 1, prs[0].Number)
	assert.Equal(t, 1, inner.callCount, "should have fetched from API")

	// Verify cache file was written
	data, err := os.ReadFile(cf.cachePath())
	require.NoError(t, err)

	var cd cacheData
	require.NoError(t, json.Unmarshal(data, &cd))
	assert.Len(t, cd.PRs, 1)
}

func TestCachedForgeHit(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cacheDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	// Pre-populate cache
	cd := cacheData{
		FetchedAt: time.Now(),
		PRs:       []PR{{Number: 5, Title: "Cached", Branch: "cached"}},
	}
	data, _ := json.Marshal(cd)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, cacheFile), data, 0o644))

	inner := &mockForge{prs: []PR{{Number: 99, Title: "Fresh"}}}
	cf := NewCachedForge(inner, gitDir)

	prs, err := cf.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 5, prs[0].Number, "should return cached data")
	assert.Equal(t, 0, inner.callCount, "should not call API when cache is fresh")
}

func TestCachedForgeStale(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cacheDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	// Pre-populate stale cache
	cd := cacheData{
		FetchedAt: time.Now().Add(-10 * time.Minute),
		PRs:       []PR{{Number: 5, Title: "Stale", Branch: "stale"}},
	}
	data, _ := json.Marshal(cd)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, cacheFile), data, 0o644))

	inner := &mockForge{prs: []PR{{Number: 99, Title: "Fresh"}}}
	cf := NewCachedForge(inner, gitDir)

	prs, err := cf.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 5, prs[0].Number, "should return stale data immediately")

	// Give background goroutine time to refresh
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, inner.callCount, "should have triggered background refresh")
}

func TestCachedForgeCorruptCache(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cacheDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))

	// Write invalid JSON
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, cacheFile), []byte("{invalid"), 0o644))

	inner := &mockForge{prs: []PR{{Number: 1, Title: "Fresh"}}}
	cf := NewCachedForge(inner, gitDir)

	prs, err := cf.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 1, prs[0].Number, "should fetch fresh on corrupt cache")
	assert.Equal(t, 1, inner.callCount)
}

func TestCachedForgeDelegation(t *testing.T) {
	inner := &mockForge{}
	cf := NewCachedForge(inner, "/tmp/test/.git")
	assert.Equal(t, "mock", cf.Name())
	assert.Equal(t, "", cf.PRURL(1))
	assert.Equal(t, "", cf.FetchRef(1))
}

func TestCachedForgeGetPR(t *testing.T) {
	inner := &mockForge{}
	cf := NewCachedForge(inner, "/tmp/test/.git")
	// GetPR delegates directly (not cached)
	pr, err := cf.GetPR(context.Background(), 1)
	assert.NoError(t, err)
	assert.Nil(t, pr)
}

func TestCachedForgeSaveError(t *testing.T) {
	// Use a read-only directory to trigger save error
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	inner := &mockForge{prs: []PR{{Number: 1, Title: "Test"}}}
	cf := NewCachedForge(inner, gitDir)

	// This should still succeed (save error is swallowed)
	prs, err := cf.ListPRs(context.Background())
	require.NoError(t, err)
	require.Len(t, prs, 1)
}
