package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// CacheTTL is how long cached PR data is considered fresh.
	CacheTTL = 5 * time.Minute

	cacheDir  = "wtf"
	cacheFile = "pr-cache.json"
)

// cacheData is the on-disk JSON format.
type cacheData struct {
	FetchedAt time.Time `json:"fetched_at"`
	PRs       []PR      `json:"prs"`
}

// CachedForge wraps a Forge and caches ListPRs results to disk.
// Stale-while-revalidate: returns stale data immediately and refreshes
// in the background when the cache is expired.
type CachedForge struct {
	inner    Forge
	cacheDir string // absolute path to .git/wtf/
	mu       sync.Mutex
}

// NewCachedForge wraps a Forge with file-based PR caching.
// gitCommonDir should be the output of `git rev-parse --git-common-dir`.
func NewCachedForge(inner Forge, gitCommonDir string) *CachedForge {
	return &CachedForge{
		inner:    inner,
		cacheDir: filepath.Join(gitCommonDir, cacheDir),
	}
}

// PRResult holds PR data from either cache or a fresh API fetch.
type PRResult struct {
	PRs       []PR
	FromCache bool
	Err       error
}

// Name returns the name of the underlying forge.
func (c *CachedForge) Name() string { return c.inner.Name() }

// PRURL returns the URL for the given PR number.
func (c *CachedForge) PRURL(number int) string { return c.inner.PRURL(number) }

// FetchRef returns the git fetch refspec for the given PR number.
func (c *CachedForge) FetchRef(number int) string { return c.inner.FetchRef(number) }

// GetPR delegates directly to the inner forge (not cached).
func (c *CachedForge) GetPR(ctx context.Context, number int) (*PR, error) {
	return c.inner.GetPR(ctx, number)
}

// ListPRs returns cached PR data if fresh, or stale data while refreshing in the background.
// On cache miss, fetches synchronously.
func (c *CachedForge) ListPRs(ctx context.Context) ([]PR, error) {
	cached, err := c.load()
	if err == nil && cached != nil {
		if time.Since(cached.FetchedAt) < CacheTTL {
			return cached.PRs, nil
		}

		// Stale — return immediately and refresh in background
		go func() {
			bgCtx := context.Background()
			prs, fetchErr := c.inner.ListPRs(bgCtx)
			if fetchErr == nil {
				_ = c.save(&cacheData{FetchedAt: time.Now(), PRs: prs})
			}
		}()
		return cached.PRs, nil
	}

	// Cache miss — fetch synchronously
	prs, err := c.inner.ListPRs(ctx)
	if err != nil {
		return nil, err
	}

	_ = c.save(&cacheData{FetchedAt: time.Now(), PRs: prs})
	return prs, nil
}

// ListPRsAsync returns cached data immediately (if available) and always
// fetches fresh data in the background. The returned channel receives up to
// two results:
//  1. Cached data (FromCache=true) — sent immediately if a cache file exists
//  2. Fresh data (FromCache=false) — sent when the API call completes
//
// The channel is closed after all results are sent.
func (c *CachedForge) ListPRsAsync(ctx context.Context) <-chan PRResult {
	ch := make(chan PRResult, 2)

	cached, _ := c.load()
	if cached != nil {
		ch <- PRResult{PRs: cached.PRs, FromCache: true}
	}

	go func() {
		defer close(ch)
		prs, err := c.inner.ListPRs(ctx)
		if err != nil {
			ch <- PRResult{Err: err}
			return
		}
		_ = c.save(&cacheData{FetchedAt: time.Now(), PRs: prs})
		ch <- PRResult{PRs: prs, FromCache: false}
	}()

	return ch
}

func (c *CachedForge) cachePath() string {
	return filepath.Join(c.cacheDir, cacheFile)
}

func (c *CachedForge) load() (*cacheData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.cachePath())
	if err != nil {
		return nil, err
	}

	var cd cacheData
	if err := json.Unmarshal(data, &cd); err != nil {
		return nil, fmt.Errorf("parsing cache: %w", err)
	}
	return &cd, nil
}

func (c *CachedForge) save(cd *cacheData) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	return os.WriteFile(c.cachePath(), data, 0o644)
}
