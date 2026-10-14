package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/charmbracelet/x/ansi"
)

func TestPRLinkIsLimitedToTerminalOutputAndClosesWhenTruncated(t *testing.T) {
	info := &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{Number: 18, State: "OPEN", URL: "https://github.com/AndrewPBerg/wtf/pull/18"}}}
	p := humanPresentation(&bytes.Buffer{})
	if label := prPresentation(p, info); label != "#18 open" || strings.Contains(label, "\x1b") {
		t.Fatalf("redirected output contains terminal markup: %q", label)
	}
	p.Links = true
	label := prPresentation(p, info)
	if !strings.Contains(label, ansi.SetHyperlink(info.Matches[0].URL)) || !strings.HasSuffix(label, ansi.ResetHyperlink()) || !strings.Contains(label, "\uf407 #18") {
		t.Fatalf("terminal PR lacks a clickable glyph: %q", label)
	}
	for _, width := range []int{1, 3, 8} {
		truncated := ansi.Truncate(label, width, "…")
		if strings.Contains(truncated, ansi.SetHyperlink(info.Matches[0].URL)) && !strings.HasSuffix(truncated, ansi.ResetHyperlink()) {
			t.Fatalf("truncation leaves the link open: %q", truncated)
		}
	}
	for _, invalid := range []string{"https://github.com/owner/repo/pull/18\x1b[31m", "https://github.com.attacker.test/pull/18", "javascript:alert(1)"} {
		info.Matches[0].URL = invalid
		if strings.Contains(prPresentation(p, info), "\x1b") {
			t.Fatalf("unsafe link accepted: %q", invalid)
		}
	}
}

func TestAbsentMetadataLabelsDistinguishFreshStaleAndFailedLookups(t *testing.T) {
	for _, test := range []struct {
		info   core.PRInfo
		pr, ci string
	}{
		{core.PRInfo{Lookup: "fresh", PRState: core.MetadataNone, CIState: core.MetadataNone}, "", ""},
		{core.PRInfo{Lookup: "stale", PRState: core.MetadataNone, CIState: core.MetadataNone}, "", ""},
		{core.PRInfo{Lookup: "cached", Error: "offline", PRState: core.MetadataNone, CIState: core.MetadataNone}, "", ""},
		{core.PRInfo{Lookup: "unavailable", Error: "offline", PRState: core.MetadataUnavailable, CIState: core.MetadataUnavailable}, "unavailable", "?"},
		{core.PRInfo{Lookup: "loading", PRState: core.MetadataLoading, CIState: core.MetadataLoading}, "loading…", "↻"},
	} {
		if pr, ci := prLabel(&test.info), ciLabel(&test.info); pr != test.pr || ci != test.ci {
			t.Fatalf("%+v: got %q/%q, want %q/%q", test.info, pr, ci, test.pr, test.ci)
		}
	}
}

func TestPRGlyphTracksDraftOpenClosedAndMergedState(t *testing.T) {
	p := humanPresentation(&bytes.Buffer{})
	p.Links = true
	for _, test := range []struct {
		state, glyph string
		draft        bool
	}{
		{"OPEN", "\uf407", false},
		{"OPEN", "\uf4dd", true},
		{"CLOSED", "\uf4dc", false},
		{"CLOSED", "\uf4dc", true},
		{"MERGED", "\uf419", false},
	} {
		info := &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{Number: 19, State: test.state, Draft: test.draft, URL: "https://github.com/AndrewPBerg/wtf/pull/19"}}}
		label := prPresentation(p, info)
		if !strings.Contains(label, test.glyph+" #19") {
			t.Fatalf("wrong icon for %s draft=%v: %q", test.state, test.draft, label)
		}
	}
}

func TestPRKeepsSameAppearanceWhileLinkingToLinearOrGitHub(t *testing.T) {
	p := humanPresentation(&bytes.Buffer{})
	p.Links = true
	info := &core.PRInfo{Matches: []core.PullRequest{{Number: 19, State: "OPEN", URL: "https://github.com/AndrewPBerg/wtf/pull/19"}}}
	github := prPresentation(p, info)
	for i := range info.Matches {
		info.Matches[i].Review = &core.ReviewResolution{Provider: "linear", URL: "https://linear.app/cascadinglabs/review/example"}
	}
	linear := prPresentation(p, info)
	if ansi.Strip(github) != ansi.Strip(linear) || !strings.Contains(linear, ansi.SetHyperlink("https://linear.app/cascadinglabs/review/example")) {
		t.Fatalf("destination changed the visible PR appearance: %q %q", github, linear)
	}
	for i := range info.Matches {
		info.Matches[i].Review.Provider = "github"
	}
	if got := prPresentation(p, info); !strings.Contains(got, ansi.SetHyperlink(info.Matches[0].URL)) {
		t.Fatal("fallback did not use the original GitHub PR URL")
	}
}

func TestCICountsUsePublishedHeadAndKeepPendingOrUnknownResultsVisible(t *testing.T) {
	for _, test := range []struct {
		checks *core.CheckSummary
		want   string
	}{
		{nil, "?"},
		{&core.CheckSummary{Head: "head"}, ""},
		{&core.CheckSummary{Head: "head", Total: 3, Failed: 1, Pending: 2}, "✗ 1 ↻ 2"},
		{&core.CheckSummary{Head: "head", Total: 3, Passed: 3}, "✓ 3"},
		{&core.CheckSummary{Head: "head", Total: 2, Skipped: 2}, "⊘ 2"},
		{&core.CheckSummary{Head: "head", Total: 1, Cancelled: 1}, "⊘ 1"},
		{&core.CheckSummary{Head: "head", Total: 1, Unknown: 1}, "? 1"},
		{&core.CheckSummary{Head: "head", Total: 100, Passed: 100, Truncated: true}, "?"},
		{&core.CheckSummary{Head: "old-head", Total: 1, Passed: 1}, "?"},
	} {
		info := &core.PRInfo{Lookup: "fresh", Matches: []core.PullRequest{{Head: "head", Checks: test.checks, URL: "https://github.com/AndrewPBerg/wtf/pull/19"}}}
		if got := ciLabel(info); got != test.want {
			t.Fatalf("checks %+v rendered %q; want %q", test.checks, got, test.want)
		}
	}
	info := &core.PRInfo{Lookup: "stale", Matches: []core.PullRequest{{Head: "head", Checks: &core.CheckSummary{Head: "head", Total: 1, Failed: 1}, URL: "https://github.com/AndrewPBerg/wtf/pull/19"}}}
	if ciLabel(info) != "✗ 1 ~" {
		t.Fatal("stale failure was presented as a live result")
	}
	p := humanPresentation(&bytes.Buffer{})
	p.Links = true
	if label := ciPresentation(p, info); !strings.Contains(label, "/pull/19/checks") || !strings.HasSuffix(label, ansi.ResetHyperlink()) {
		t.Fatalf("CI summary is not linked to checks: %q", label)
	}
}
