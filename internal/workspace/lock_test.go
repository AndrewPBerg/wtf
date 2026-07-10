package workspace

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithLockSerializesConcurrentCallers(t *testing.T) {
	root := t.TempDir()
	var running, maxRunning int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, withLock(root, func() error {
				mu.Lock()
				running++
				if running > maxRunning {
					maxRunning = running
				}
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				mu.Lock()
				running--
				mu.Unlock()
				return nil
			}))
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, maxRunning)
}
