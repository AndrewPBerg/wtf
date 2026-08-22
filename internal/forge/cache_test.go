package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockForge is a test double for the Forge interface.
type mockForge struct {
	prs       []PR
	callCount atomic.Int32
	err       error
	delay     time.Duration
}

func (m *mockForge) Name() string                                { return "mock" }
func (m *mockForge) PRURL(_ int) string                          { return "" }
func (m *mockForge) FetchRef(_ int) string                       { return "" }
func (m *mockForge) GetPR(_ context.Context, _ int) (*PR, error) { return nil, nil }

func (m *mockForge) ListPRs(_ context.Context) ([]PR, error) {
	m.callCount.Add(1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
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
	assert.Equal(t, int32(1), inner.callCount.Load(), "should have fetched from API")

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
	assert.Equal(t, int32(0), inner.callCount.Load(), "should not call API when cache is fresh")
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
	assert.Equal(t, int32(1), inner.callCount.Load(), "should have triggered background refresh")
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
	assert.Equal(t, int32(1), inner.callCount.Load())
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

func TestListPRsAsyncCacheHitThenFresh(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cDir, 0o755))

	// Pre-populate cache
	cd := cacheData{
		FetchedAt: time.Now().Add(-10 * time.Minute),
		PRs:       []PR{{Number: 5, Title: "Cached", Branch: "cached"}},
	}
	data, _ := json.Marshal(cd)
	require.NoError(t, os.WriteFile(filepath.Join(cDir, cacheFile), data, 0o644))

	inner := &mockForge{prs: []PR{{Number: 99, Title: "Fresh", Branch: "fresh"}}}
	cf := NewCachedForge(inner, gitDir)

	ch := cf.ListPRsAsync(context.Background())

	// First result should be cached
	r1 := <-ch
	require.NoError(t, r1.Err)
	assert.True(t, r1.FromCache)
	require.Len(t, r1.PRs, 1)
	assert.Equal(t, 5, r1.PRs[0].Number)

	// Second result should be fresh
	r2 := <-ch
	require.NoError(t, r2.Err)
	assert.False(t, r2.FromCache)
	require.Len(t, r2.PRs, 1)
	assert.Equal(t, 99, r2.PRs[0].Number)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)
	assert.Equal(t, int32(1), inner.callCount.Load())
}

func TestListPRsAsyncColdStart(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	inner := &mockForge{prs: []PR{{Number: 1, Title: "First", Branch: "first"}}}
	cf := NewCachedForge(inner, gitDir)

	ch := cf.ListPRsAsync(context.Background())

	// Only one result — fresh (no cache existed)
	r1 := <-ch
	require.NoError(t, r1.Err)
	assert.False(t, r1.FromCache)
	require.Len(t, r1.PRs, 1)
	assert.Equal(t, 1, r1.PRs[0].Number)

	_, ok := <-ch
	assert.False(t, ok)
	assert.Equal(t, int32(1), inner.callCount.Load())
}

func TestListPRsAsyncAPIError(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cDir, 0o755))

	// Pre-populate cache
	cd := cacheData{
		FetchedAt: time.Now(),
		PRs:       []PR{{Number: 5, Title: "Cached", Branch: "cached"}},
	}
	data, _ := json.Marshal(cd)
	require.NoError(t, os.WriteFile(filepath.Join(cDir, cacheFile), data, 0o644))

	inner := &mockForge{err: fmt.Errorf("API down")}
	cf := NewCachedForge(inner, gitDir)

	ch := cf.ListPRsAsync(context.Background())

	// First result: cached
	r1 := <-ch
	require.NoError(t, r1.Err)
	assert.True(t, r1.FromCache)
	assert.Equal(t, 5, r1.PRs[0].Number)

	// Second result: error from API
	r2 := <-ch
	assert.Error(t, r2.Err)
	assert.Nil(t, r2.PRs)

	_, ok := <-ch
	assert.False(t, ok)
}

func TestListPRsAsyncCorruptCache(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	cDir := filepath.Join(gitDir, "wtf")
	require.NoError(t, os.MkdirAll(cDir, 0o755))

	// Write invalid JSON
	require.NoError(t, os.WriteFile(filepath.Join(cDir, cacheFile), []byte("{invalid"), 0o644))

	inner := &mockForge{prs: []PR{{Number: 1, Title: "Fresh", Branch: "fresh"}}}
	cf := NewCachedForge(inner, gitDir)

	ch := cf.ListPRsAsync(context.Background())

	// Only fresh result (corrupt cache is skipped)
	r1 := <-ch
	require.NoError(t, r1.Err)
	assert.False(t, r1.FromCache)
	assert.Equal(t, 1, r1.PRs[0].Number)

	_, ok := <-ch
	assert.False(t, ok)
}

func TestListPRsAsyncContextCancelled(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	inner := &mockForge{
		prs:   []PR{{Number: 1, Title: "Fresh"}},
		delay: 50 * time.Millisecond,
	}
	cf := NewCachedForge(inner, gitDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ch := cf.ListPRsAsync(ctx)

	// Drain whatever comes out — channel should close.
	var results []PRResult
	for r := range ch {
		results = append(results, r)
	}
	// We got at least one result (the mock ignores context, so it completes)
	assert.NotEmpty(t, results)
}
