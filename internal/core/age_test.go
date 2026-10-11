package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkspaceRegistrationTimePersistsAcrossSetupAndObservations(t *testing.T) {
	a, r, anchor := fixture(t)
	if anchor.CreatedAt == nil || anchor.CreatedAt.IsZero() || anchor.CreatedAt.After(time.Now()) {
		t.Fatalf("registration time missing or invalid: %+v", anchor)
	}
	child, err := a.NewWorkspace(t.Context(), "../age-feature", "@")
	if err != nil || child.CreatedAt == nil || child.CreatedAt.IsZero() {
		t.Fatalf("creation time missing: %+v %v", child, err)
	}
	for _, want := range []Workspace{anchor, child.Workspace} {
		repeated, err := a.NewWorkspace(t.Context(), want.Path, "")
		if err != nil || repeated.CreatedAt == nil || !repeated.CreatedAt.Equal(*want.CreatedAt) {
			t.Fatalf("setup reset age: %+v %v", repeated, err)
		}
	}
	state, err := r.Load()
	if err != nil {
		t.Fatal(err)
	}
	rows := views(state)
	inspector, err := a.RemovalInspector(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		want := state.Workspaces[row.ID].CreatedAt
		if row.CreatedAt == nil || !row.CreatedAt.Equal(*want) {
			t.Fatal("list lost registration time")
		}
		candidate, err := inspector.Inspect(t.Context(), row)
		if err != nil || candidate.CreatedAt == nil || !candidate.CreatedAt.Equal(*want) {
			t.Fatalf("removal inspection lost registration time: %+v %v", candidate, err)
		}
		data, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			CreatedAt time.Time `json:"created_at"`
		}
		if err := json.Unmarshal(data, &decoded); err != nil || !decoded.CreatedAt.Equal(*want) {
			t.Fatalf("JSON lost registration time: %s %v", data, err)
		}
	}
}

func TestLegacyWorkspaceAgeRemainsUnknownAfterSetup(t *testing.T) {
	a, r, anchor := fixture(t)
	if err := r.Update(t.Context(), anchor.ID, func(w *Workspace) { w.CreatedAt = nil }); err != nil {
		t.Fatal(err)
	}
	repeated, err := a.NewWorkspace(t.Context(), anchor.Path, "")
	if err != nil || repeated.CreatedAt != nil {
		t.Fatalf("legacy age was backfilled: %+v %v", repeated, err)
	}
	state, err := r.Load()
	if err != nil || state.Workspaces[anchor.ID].CreatedAt != nil {
		t.Fatalf("legacy state changed: %+v %v", state, err)
	}
	data, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["created_at"]; exists {
		t.Fatalf("unknown time should be absent: %s", data)
	}
}
