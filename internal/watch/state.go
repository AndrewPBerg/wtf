package watch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndrewPBerg/wtf/internal/forge"
)

const stateFile = "watch-state.json"

// PRSnapshot stores the fields we track for change detection.
// Title is not used for diffing but is kept for display when GetPR fails.
type PRSnapshot struct {
	Title        string             `json:"title"`
	ReviewStatus forge.ReviewStatus `json:"review_status"`
	IsDraft      bool               `json:"is_draft"`
}

// State holds the last-known PR state for a repository.
type State struct {
	RemoteURL string             `json:"remote_url,omitempty"`
	PRs       map[int]PRSnapshot `json:"prs"`
}

// SnapshotPRs converts a list of open PRs into a State.
func SnapshotPRs(prs []forge.PR) State {
	s := State{PRs: make(map[int]PRSnapshot, len(prs))}
	for _, pr := range prs {
		s.PRs[pr.Number] = PRSnapshot{
			Title:        pr.Title,
			ReviewStatus: pr.ReviewStatus,
			IsDraft:      pr.IsDraft,
		}
	}
	return s
}

// LoadState reads the watch state from the given directory.
// Returns a zero State (nil PRs map) if the file does not exist.
func LoadState(dir string) (State, error) {
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("reading watch state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parsing watch state: %w", err)
	}
	return s, nil
}

// SaveState writes the watch state to the given directory.
func SaveState(dir string, s State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling watch state: %w", err)
	}

	if err := os.WriteFile(statePath(dir), data, 0o600); err != nil {
		return fmt.Errorf("writing watch state: %w", err)
	}
	return nil
}

// IsFirstRun returns true if the state has never been populated.
func (s State) IsFirstRun() bool {
	return s.PRs == nil
}

func statePath(dir string) string {
	return filepath.Join(dir, stateFile)
}
