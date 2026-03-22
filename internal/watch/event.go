package watch

import (
	"fmt"

	"github.com/AndrewPBerg/wtf/internal/forge"
)

// EventKind identifies what changed in a PR.
type EventKind string

// Event kinds for PR state changes.
const (
	EventNewPR    EventKind = "new_pr"
	EventPRClosed EventKind = "pr_closed"
	EventPRMerged EventKind = "pr_merged"
	EventReview   EventKind = "review_changed"
	EventDraft    EventKind = "draft_changed"
)

// Event represents a detected change in PR state.
type Event struct {
	Kind   EventKind `json:"kind"`
	PR     forge.PR  `json:"pr"`
	Repo   string    `json:"repo,omitempty"`
	Detail string    `json:"detail"`
}

// String returns a human-readable description of the event.
func (e Event) String() string {
	switch e.Kind {
	case EventNewPR:
		return fmt.Sprintf("New PR #%d %q by %s", e.PR.Number, e.PR.Title, e.PR.Author)
	case EventPRClosed:
		return fmt.Sprintf("PR #%d %q closed", e.PR.Number, e.PR.Title)
	case EventPRMerged:
		return fmt.Sprintf("PR #%d %q merged", e.PR.Number, e.PR.Title)
	case EventReview:
		return fmt.Sprintf("PR #%d %q %s", e.PR.Number, e.PR.Title, e.Detail)
	case EventDraft:
		return fmt.Sprintf("PR #%d %q %s", e.PR.Number, e.PR.Title, e.Detail)
	default:
		return fmt.Sprintf("PR #%d: %s", e.PR.Number, e.Detail)
	}
}

// Diff compares old state against current open PRs and returns events.
// It does NOT detect merged vs closed — disappeared PRs are returned
// separately via the second return value (their PR numbers) so the caller
// can look them up via GetPR.
func Diff(old State, current []forge.PR) (events []Event, disappeared []int) {
	currentMap := make(map[int]forge.PR, len(current))
	for _, pr := range current {
		currentMap[pr.Number] = pr
	}

	// Detect new PRs and state changes.
	for _, pr := range current {
		snap, exists := old.PRs[pr.Number]
		if !exists {
			events = append(events, Event{
				Kind:   EventNewPR,
				PR:     pr,
				Detail: fmt.Sprintf("opened by %s", pr.Author),
			})
			continue
		}

		// Review status changed.
		if pr.ReviewStatus != snap.ReviewStatus && pr.ReviewStatus != "" {
			events = append(events, Event{
				Kind:   EventReview,
				PR:     pr,
				Detail: string(pr.ReviewStatus),
			})
		}

		// Draft status changed.
		if pr.IsDraft != snap.IsDraft {
			detail := "marked as draft"
			if !pr.IsDraft {
				detail = "marked as ready for review"
			}
			events = append(events, Event{
				Kind:   EventDraft,
				PR:     pr,
				Detail: detail,
			})
		}
	}

	// Detect disappeared PRs (closed or merged).
	for num := range old.PRs {
		if _, exists := currentMap[num]; !exists {
			disappeared = append(disappeared, num)
		}
	}

	return events, disappeared
}

// ClassifyDisappeared returns a closed or merged event for a PR that
// disappeared from the open list. Use the PR returned by GetPR.
func ClassifyDisappeared(pr forge.PR) Event {
	if pr.State == forge.PRMerged {
		return Event{
			Kind:   EventPRMerged,
			PR:     pr,
			Detail: "merged",
		}
	}
	return Event{
		Kind:   EventPRClosed,
		PR:     pr,
		Detail: "closed",
	}
}
