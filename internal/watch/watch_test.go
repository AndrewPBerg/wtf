package watch

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockForge implements forge.Forge for testing.
type mockForge struct {
	mu      sync.Mutex
	prLists [][]forge.PR // successive results for ListPRs
	callIdx int
	getPR   map[int]*forge.PR
}

func (m *mockForge) ListPRs(_ context.Context) ([]forge.PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.prLists) {
		return m.prLists[len(m.prLists)-1], nil
	}
	prs := m.prLists[m.callIdx]
	m.callIdx++
	return prs, nil
}

func (m *mockForge) GetPR(_ context.Context, number int) (*forge.PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pr, ok := m.getPR[number]; ok {
		return pr, nil
	}
	return &forge.PR{Number: number, State: forge.PRClosed}, nil
}

func (m *mockForge) PRURL(_ int) string    { return "" }
func (m *mockForge) FetchRef(_ int) string { return "" }
func (m *mockForge) Name() string          { return "mock" }

// mockNotifier records notifications.
type mockNotifier struct {
	mu      sync.Mutex
	entries []string
}

func (m *mockNotifier) Notify(_ context.Context, title, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, title+": "+body)
	return nil
}

func (m *mockNotifier) Name() string { return "mock" }

func (m *mockNotifier) get() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.entries))
	copy(result, m.entries)
	return result
}

func TestWatcher_FirstRun_Silent(t *testing.T) {
	dir := t.TempDir()
	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 1, Title: "feat", Author: "alice"}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := w.Run(ctx)
	require.NoError(t, err)

	// No notifications on first run.
	assert.Empty(t, mn.get())

	// State should be saved.
	state, err := LoadState(dir)
	require.NoError(t, err)
	assert.Len(t, state.PRs, 1)

	// Log should mention watching.
	assert.Contains(t, buf.String(), "Watching 1 open PRs")
}

func TestWatcher_DetectsNewPR(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate state so first run isn't silent.
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{
		1: {Title: "existing"},
	}})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			// First poll returns existing + new PR.
			{
				{Number: 1, Title: "existing"},
				{Number: 2, Title: "new feat", Author: "bob"},
			},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	entries := mn.get()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0], "new feat")
}

func TestWatcher_DetectsDisappearedPR(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{
		1: {Title: "will merge"},
	}})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			{}, // PR disappeared.
		},
		getPR: map[int]*forge.PR{
			1: {Number: 1, Title: "will merge", State: forge.PRMerged},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	entries := mn.get()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0], "merged")
}

func TestWatcher_RepoNameInNotification(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{}})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 1, Title: "new", Author: "alice"}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
		WithRepoName("my-repo"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	entries := mn.get()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0], "my-repo")
}

func TestWatcher_PollError_ContinuesRunning(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{}})
	require.NoError(t, err)

	var callCount atomic.Int32
	mf := &mockForgeWithError{
		onListPRs: func() ([]forge.PR, error) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, fmt.Errorf("transient API error")
			}
			return []forge.PR{{Number: 1, Title: "recovered", Author: "alice"}}, nil
		},
		getPR: map[int]*forge.PR{},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	// Should log the error but continue.
	assert.Contains(t, buf.String(), "poll error")
	// Should eventually detect the new PR.
	entries := mn.get()
	assert.NotEmpty(t, entries)
}

func TestWatcher_ReviewStatusChange(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{
		1: {Title: "feat", ReviewStatus: forge.ReviewPending},
	}})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 1, Title: "feat", ReviewStatus: forge.ReviewApproved}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	entries := mn.get()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0], "approved")
}

func TestColorForRepo(t *testing.T) {
	tests := []struct {
		name string
		repo string
	}{
		{"simple name", "my-repo"},
		{"another name", "wtf"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ColorForRepo(tt.repo)
			// Should return a valid color from the palette.
			found := false
			for _, rc := range repoColors {
				if c == rc {
					found = true
					break
				}
			}
			assert.True(t, found, "color should be from repoColors palette")
		})
	}

	// Same name always gives same color.
	assert.Equal(t, ColorForRepo("foo"), ColorForRepo("foo"))
}

func TestColorForRepo_Deterministic(t *testing.T) {
	// Same name always gives same color across calls.
	assert.Equal(t, ColorForRepo("repo-alpha"), ColorForRepo("repo-alpha"))
	assert.Equal(t, ColorForRepo("repo-beta"), ColorForRepo("repo-beta"))
}

func TestWithRepoColor(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{}})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 1, Title: "new", Author: "alice", URL: "https://github.com/org/repo/pull/1"}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	c := ColorForRepo("test-repo")
	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
		WithRepoName("test-repo"),
		WithRepoColor(c),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	// Log should contain the repo name and PR URL.
	output := buf.String()
	assert.Contains(t, output, "test-repo")
	assert.Contains(t, output, "https://github.com/org/repo/pull/1")
}

func TestWatcher_PollErrorSilencedAfterFirst(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{}})
	require.NoError(t, err)

	var callCount atomic.Int32
	mf := &mockForgeWithError{
		onListPRs: func() ([]forge.PR, error) {
			callCount.Add(1)
			return nil, fmt.Errorf("persistent error")
		},
		getPR: map[int]*forge.PR{},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	// Should only see "poll error" once despite multiple failures.
	output := buf.String()
	count := 0
	for i := 0; i < len(output); i++ {
		idx := bytes.Index([]byte(output[i:]), []byte("poll error"))
		if idx < 0 {
			break
		}
		count++
		i += idx + len("poll error")
	}
	assert.Equal(t, 1, count, "poll error should only be logged once")
}

// mockForgeWithError allows custom ListPRs behavior including errors.
type mockForgeWithError struct {
	mu        sync.Mutex
	onListPRs func() ([]forge.PR, error)
	getPR     map[int]*forge.PR
}

func (m *mockForgeWithError) ListPRs(_ context.Context) ([]forge.PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.onListPRs()
}

func (m *mockForgeWithError) GetPR(_ context.Context, number int) (*forge.PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pr, ok := m.getPR[number]; ok {
		return pr, nil
	}
	return &forge.PR{Number: number, State: forge.PRClosed}, nil
}

func (m *mockForgeWithError) PRURL(_ int) string    { return "" }
func (m *mockForgeWithError) FetchRef(_ int) string { return "" }
func (m *mockForgeWithError) Name() string          { return "mock" }

func TestWatcher_GetPR_FailsForDisappeared(t *testing.T) {
	dir := t.TempDir()
	err := SaveState(dir, State{PRs: map[int]PRSnapshot{
		1: {Title: "will fail lookup"},
	}})
	require.NoError(t, err)

	mf := &mockForgeGetPRError{
		prLists: [][]forge.PR{{}},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	entries := mn.get()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0], "closed")
}

// mockForgeGetPRError fails on GetPR calls.
type mockForgeGetPRError struct {
	mu      sync.Mutex
	prLists [][]forge.PR
	callIdx int
}

func (m *mockForgeGetPRError) ListPRs(_ context.Context) ([]forge.PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.prLists) {
		return m.prLists[len(m.prLists)-1], nil
	}
	prs := m.prLists[m.callIdx]
	m.callIdx++
	return prs, nil
}

func (m *mockForgeGetPRError) GetPR(_ context.Context, number int) (*forge.PR, error) {
	return nil, fmt.Errorf("API error looking up PR #%d", number)
}

func (m *mockForgeGetPRError) PRURL(_ int) string    { return "" }
func (m *mockForgeGetPRError) FetchRef(_ int) string { return "" }
func (m *mockForgeGetPRError) Name() string          { return "mock" }

func TestWithRemoteURL(t *testing.T) {
	dir := t.TempDir()
	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 1, Title: "feat"}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
		WithRemoteURL("https://github.com/org/repo.git"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := w.Run(ctx)
	require.NoError(t, err)

	// State should have the remote URL stamped
	state, err := LoadState(dir)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/org/repo.git", state.RemoteURL)
}

func TestWatcher_RemoteURLReset(t *testing.T) {
	dir := t.TempDir()
	// Save state with a different remote URL
	err := SaveState(dir, State{
		RemoteURL: "https://github.com/old/repo.git",
		PRs:       map[int]PRSnapshot{1: {Title: "old"}},
	})
	require.NoError(t, err)

	mf := &mockForge{
		prLists: [][]forge.PR{
			{{Number: 2, Title: "new"}},
		},
	}
	mn := &mockNotifier{}
	var buf bytes.Buffer

	w := New(mf, mn, dir,
		WithInterval(10*time.Millisecond),
		WithLogger(&buf),
		WithRemoteURL("https://github.com/new/repo.git"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = w.Run(ctx)
	require.NoError(t, err)

	// Should log remote change and reset
	assert.Contains(t, buf.String(), "Remote changed")

	// State should be fresh with new remote
	state, err := LoadState(dir)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/new/repo.git", state.RemoteURL)
	assert.Len(t, state.PRs, 1)
	_, ok := state.PRs[2]
	assert.True(t, ok)
}
