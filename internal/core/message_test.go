package core

import "testing"

func TestWorkspaceMessagePersistsAndCanBeUpdatedOrCleared(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `return {overrides={init=function(ctx,request,default)
 default(request)
 ctx:require(ctx.workspace.message == request.args[1], "Workspace message lost")
end}}`)
	message := "Hello this Is Me Testing 日本"
	child, err := a.NewWorkspaceWithOptions(t.Context(), "../described", "@", NewWorkspaceOptions{Message: &message}, message)
	if err != nil || child.Message != message {
		t.Fatalf("new message missing: %+v %v", child, err)
	}
	createdAt := *child.CreatedAt
	for _, test := range []struct {
		message *string
		want    string
	}{
		{nil, message},
		{messageString("Updated Description"), "Updated Description"},
		{messageString(""), ""},
	} {
		repeated, err := a.NewWorkspaceWithOptions(t.Context(), child.Path, "", NewWorkspaceOptions{Message: test.message}, test.want)
		if err != nil || repeated.Message != test.want || !repeated.CreatedAt.Equal(createdAt) {
			t.Fatalf("message update/reset failed: %+v %v", repeated, err)
		}
		state, err := r.Load()
		if err != nil || state.Workspaces[child.ID].Message != test.want {
			t.Fatalf("message did not persist: %+v %v", state, err)
		}
	}
}

func messageString(value string) *string { return &value }

func TestWorkspaceMessageSurvivesFailedPreparation(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `return {overrides={init=function(ctx,request,default)
 default(request)
 ctx:block("Preparation failed")
end}}`)
	message := "Retry this setup"
	child, err := a.NewWorkspaceWithOptions(t.Context(), "../retry-message", "@", NewWorkspaceOptions{Message: &message})
	if err == nil || child.Message != message {
		t.Fatalf("failed setup lost message: %+v %v", child, err)
	}
	state, err := r.Load()
	if err != nil || state.Workspaces[child.ID].Message != message {
		t.Fatalf("failure record lost message: %+v %v", state, err)
	}
}

func TestInvalidWorkspaceMessageDoesNotMutateRegistration(t *testing.T) {
	a, r, anchor := fixture(t)
	invalid := "bad\x00message"
	_, err := a.NewWorkspaceWithOptions(t.Context(), anchor.Path, "", NewWorkspaceOptions{Message: &invalid})
	if ErrorCode(err) != "invalid_message" {
		t.Fatalf("invalid message accepted: %v", err)
	}
	state, err := r.Load()
	if err != nil || state.Workspaces[anchor.ID].Message != "" {
		t.Fatalf("invalid message mutated state: %+v %v", state, err)
	}
}
