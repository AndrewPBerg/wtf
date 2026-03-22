package watch

import (
	"testing"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/stretchr/testify/assert"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name            string
		old             State
		current         []forge.PR
		wantEvents      []EventKind
		wantDisappeared []int
	}{
		{
			name: "new PR detected",
			old: State{
				PRs: map[int]PRSnapshot{},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", Author: "alice"},
			},
			wantEvents:      []EventKind{EventNewPR},
			wantDisappeared: nil,
		},
		{
			name: "PR disappeared",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "old PR"},
				},
			},
			current:         []forge.PR{},
			wantEvents:      nil,
			wantDisappeared: []int{1},
		},
		{
			name: "review status changed",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "feat", ReviewStatus: forge.ReviewPending},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", ReviewStatus: forge.ReviewApproved},
			},
			wantEvents:      []EventKind{EventReview},
			wantDisappeared: nil,
		},
		{
			name: "draft toggled to ready",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "feat", IsDraft: true},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", IsDraft: false},
			},
			wantEvents:      []EventKind{EventDraft},
			wantDisappeared: nil,
		},
		{
			name: "draft toggled to draft",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "feat", IsDraft: false},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", IsDraft: true},
			},
			wantEvents:      []EventKind{EventDraft},
			wantDisappeared: nil,
		},
		{
			name: "no changes",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "feat", ReviewStatus: forge.ReviewPending, IsDraft: false},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", ReviewStatus: forge.ReviewPending, IsDraft: false},
			},
			wantEvents:      nil,
			wantDisappeared: nil,
		},
		{
			name: "multiple changes at once",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "old"},
					2: {Title: "will disappear"},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "old", ReviewStatus: forge.ReviewApproved},
				{Number: 3, Title: "brand new", Author: "bob"},
			},
			wantEvents:      []EventKind{EventReview, EventNewPR},
			wantDisappeared: []int{2},
		},
		{
			name: "empty review status change ignored",
			old: State{
				PRs: map[int]PRSnapshot{
					1: {Title: "feat", ReviewStatus: forge.ReviewPending},
				},
			},
			current: []forge.PR{
				{Number: 1, Title: "feat", ReviewStatus: ""},
			},
			wantEvents:      nil,
			wantDisappeared: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, disappeared := Diff(tt.old, tt.current)

			var kinds []EventKind
			for _, e := range events {
				kinds = append(kinds, e.Kind)
			}
			assert.Equal(t, tt.wantEvents, kinds)
			assert.Equal(t, tt.wantDisappeared, disappeared)
		})
	}
}

func TestClassifyDisappeared(t *testing.T) {
	tests := []struct {
		name string
		pr   forge.PR
		want EventKind
	}{
		{
			name: "merged PR",
			pr:   forge.PR{Number: 1, Title: "feat", State: forge.PRMerged},
			want: EventPRMerged,
		},
		{
			name: "closed PR",
			pr:   forge.PR{Number: 2, Title: "wontfix", State: forge.PRClosed},
			want: EventPRClosed,
		},
		{
			name: "unknown state defaults to closed",
			pr:   forge.PR{Number: 3, Title: "unknown"},
			want: EventPRClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ClassifyDisappeared(tt.pr)
			assert.Equal(t, tt.want, event.Kind)
		})
	}
}

func TestEvent_String(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "new PR",
			event: Event{
				Kind: EventNewPR,
				PR:   forge.PR{Number: 42, Title: "Add watch", Author: "alice"},
			},
			want: `New PR #42 "Add watch" by alice`,
		},
		{
			name: "merged",
			event: Event{
				Kind: EventPRMerged,
				PR:   forge.PR{Number: 10, Title: "Big change"},
			},
			want: `PR #10 "Big change" merged`,
		},
		{
			name: "closed",
			event: Event{
				Kind: EventPRClosed,
				PR:   forge.PR{Number: 5, Title: "Rejected"},
			},
			want: `PR #5 "Rejected" closed`,
		},
		{
			name: "review",
			event: Event{
				Kind:   EventReview,
				PR:     forge.PR{Number: 7, Title: "Fix"},
				Detail: "approved",
			},
			want: `PR #7 "Fix" approved`,
		},
		{
			name: "draft changed",
			event: Event{
				Kind:   EventDraft,
				PR:     forge.PR{Number: 8, Title: "WIP"},
				Detail: "marked as draft",
			},
			want: `PR #8 "WIP" marked as draft`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.event.String())
		})
	}
}
