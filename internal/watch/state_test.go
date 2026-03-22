package watch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotPRs(t *testing.T) {
	prs := []forge.PR{
		{Number: 1, Title: "feat", ReviewStatus: forge.ReviewApproved, IsDraft: false},
		{Number: 2, Title: "fix", ReviewStatus: forge.ReviewPending, IsDraft: true},
	}

	s := SnapshotPRs(prs)
	assert.Len(t, s.PRs, 2)
	assert.Equal(t, "feat", s.PRs[1].Title)
	assert.Equal(t, forge.ReviewApproved, s.PRs[1].ReviewStatus)
	assert.False(t, s.PRs[1].IsDraft)
	assert.Equal(t, "fix", s.PRs[2].Title)
	assert.True(t, s.PRs[2].IsDraft)
}

func TestLoadState_NoFile(t *testing.T) {
	s, err := LoadState(t.TempDir())
	require.NoError(t, err)
	assert.True(t, s.IsFirstRun())
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	original := State{
		PRs: map[int]PRSnapshot{
			1: {Title: "feat", ReviewStatus: forge.ReviewApproved, IsDraft: false},
			2: {Title: "fix", ReviewStatus: forge.ReviewPending, IsDraft: true},
		},
	}

	err := SaveState(dir, original)
	require.NoError(t, err)

	loaded, err := LoadState(dir)
	require.NoError(t, err)
	assert.Equal(t, original, loaded)
}

func TestLoadState_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, stateFile), []byte("not json"), 0o644)
	require.NoError(t, err)

	_, err = LoadState(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing watch state")
}

func TestSaveState_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	s := State{PRs: map[int]PRSnapshot{}}

	err := SaveState(dir, s)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, stateFile))
	assert.NoError(t, err)
}

func TestSaveState_UnwritableDir(t *testing.T) {
	err := SaveState("/dev/null/impossible", State{PRs: map[int]PRSnapshot{}})
	assert.Error(t, err)
}

func TestState_IsFirstRun(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{
			name:  "nil PRs map",
			state: State{},
			want:  true,
		},
		{
			name:  "empty PRs map",
			state: State{PRs: map[int]PRSnapshot{}},
			want:  false,
		},
		{
			name: "populated PRs map",
			state: State{PRs: map[int]PRSnapshot{
				1: {Title: "feat"},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.IsFirstRun())
		})
	}
}
