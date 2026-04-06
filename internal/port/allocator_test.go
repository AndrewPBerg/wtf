package port

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failStore always returns an error on Load.
type failStore struct{}

func (f *failStore) Load() (map[string]int, error) { return nil, fmt.Errorf("disk on fire") }
func (f *failStore) Save(map[string]int) error     { return nil }

func newTestAllocator(t *testing.T, base int) *Allocator {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ports.json")
	return New(base, NewFileStore(path))
}

func TestAllocate_FirstGetBase(t *testing.T) {
	a := newTestAllocator(t, 8080)
	p, err := a.Allocate("main")
	require.NoError(t, err)
	assert.Equal(t, 8080, p)
}

func TestAllocate_SequentialIncrement(t *testing.T) {
	a := newTestAllocator(t, 3000)

	p1, err := a.Allocate("main")
	require.NoError(t, err)
	assert.Equal(t, 3000, p1)

	p2, err := a.Allocate("feature/a")
	require.NoError(t, err)
	assert.Equal(t, 3001, p2)

	p3, err := a.Allocate("feature/b")
	require.NoError(t, err)
	assert.Equal(t, 3002, p3)
}

func TestAllocate_Idempotent(t *testing.T) {
	a := newTestAllocator(t, 8000)

	p1, err := a.Allocate("main")
	require.NoError(t, err)

	p2, err := a.Allocate("main")
	require.NoError(t, err)

	assert.Equal(t, p1, p2)
}

func TestRelease_FreesPort(t *testing.T) {
	a := newTestAllocator(t, 8000)

	_, err := a.Allocate("main")
	require.NoError(t, err)

	_, err = a.Allocate("feature/a")
	require.NoError(t, err)

	require.NoError(t, a.Release("main"))

	// Next allocation should reuse the freed port (8000)
	p, err := a.Allocate("feature/b")
	require.NoError(t, err)
	assert.Equal(t, 8000, p)
}

func TestRelease_NonexistentBranch(t *testing.T) {
	a := newTestAllocator(t, 8000)
	err := a.Release("nonexistent")
	assert.NoError(t, err)
}

func TestAllocate_GapFilling(t *testing.T) {
	a := newTestAllocator(t, 3000)

	_, _ = a.Allocate("a") // 3000
	_, _ = a.Allocate("b") // 3001
	_, _ = a.Allocate("c") // 3002

	require.NoError(t, a.Release("b")) // frees 3001

	p, err := a.Allocate("d")
	require.NoError(t, err)
	assert.Equal(t, 3001, p) // fills the gap
}

func TestLookup_Exists(t *testing.T) {
	a := newTestAllocator(t, 8000)

	_, err := a.Allocate("main")
	require.NoError(t, err)

	p, ok, err := a.Lookup("main")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 8000, p)
}

func TestLookup_NotFound(t *testing.T) {
	a := newTestAllocator(t, 8000)

	_, ok, err := a.Lookup("nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestListAll(t *testing.T) {
	a := newTestAllocator(t, 3000)

	_, _ = a.Allocate("main")
	_, _ = a.Allocate("feature/a")

	m, err := a.ListAll()
	require.NoError(t, err)
	assert.Equal(t, map[string]int{
		"main":      3000,
		"feature/a": 3001,
	}, m)
}

func TestListAll_Empty(t *testing.T) {
	a := newTestAllocator(t, 8000)

	m, err := a.ListAll()
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestAllocate_StoreLoadError(t *testing.T) {
	a := New(8000, &failStore{})
	_, err := a.Allocate("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading ports")
}

func TestLookup_StoreLoadError(t *testing.T) {
	a := New(8000, &failStore{})
	_, _, err := a.Lookup("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading ports")
}

func TestRelease_StoreLoadError(t *testing.T) {
	a := New(8000, &failStore{})
	err := a.Release("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading ports")
}

func TestListAll_StoreLoadError(t *testing.T) {
	a := New(8000, &failStore{})
	_, err := a.ListAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading ports")
}
