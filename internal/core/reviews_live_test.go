package core

import (
	"encoding/json"
	"os"
	"testing"
)

// TestNativeReviewLive is opt-in because it uses the user's configured MCP accounts.
func TestNativeReviewLive(t *testing.T) {
	input := os.Getenv("WTF_REVIEW_LIVE_CASES")
	if input == "" {
		t.Skip("set WTF_REVIEW_LIVE_CASES to explicitly test live account routing")
	}
	var cases []struct {
		Root      string `json:"root"`
		PR        string `json:"pr"`
		Provider  string `json:"provider"`
		Workspace string `json:"workspace"`
		Code      string `json:"code"`
	}
	if err := json.Unmarshal([]byte(input), &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		result := (NativeReviewClient{}).Resolve(t.Context(), test.Root, test.PR)
		if result.Provider != test.Provider || result.Workspace != test.Workspace {
			t.Fatalf("native lookup did not match expected provider/account: %+v", result)
		}
		if test.Code != "" && result.Code != test.Code {
			t.Fatalf("native lookup returned an unexpected outcome: %+v", result)
		}
		if result.Provider == "linear" && !validLinearReview(result.URL, result.Workspace) {
			t.Fatalf("native lookup returned invalid review URL: %+v", result)
		}
	}
}
