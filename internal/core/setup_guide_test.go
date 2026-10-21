package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSetupGuideUpgradeCases(t *testing.T) {
	old := []byte("# Earlier shipped template\n")
	custom := []byte("# Project edits\n")
	latestRecord := []byte(guideChecksum(workspaceGuide) + "\n")
	oldRecord := []byte(guideChecksum(old) + "\n")
	for _, tc := range []struct {
		name                  string
		guide, record         []byte
		updated               []string
		available             bool
		wantGuide, wantRecord []byte
	}{
		{name: "fresh", wantGuide: workspaceGuide, wantRecord: latestRecord},
		{name: "current", guide: workspaceGuide, record: latestRecord, wantGuide: workspaceGuide, wantRecord: latestRecord},
		{name: "unchanged old template", guide: old, record: oldRecord, updated: []string{".wtf/AGENTS.md", ".wtf/" + guideChecksumFile}, wantGuide: workspaceGuide, wantRecord: latestRecord},
		{name: "customized old template", guide: custom, record: oldRecord, available: true, wantGuide: custom, wantRecord: oldRecord},
		{name: "customized current template", guide: custom, record: latestRecord, available: true, wantGuide: custom, wantRecord: latestRecord},
		{name: "unknown old guide", guide: old, available: true, wantGuide: old},
		{name: "adopt exact current template", guide: workspaceGuide, wantGuide: workspaceGuide, wantRecord: latestRecord},
		{name: "recover guide written before checksum", guide: workspaceGuide, record: oldRecord, updated: []string{".wtf/" + guideChecksumFile}, wantGuide: workspaceGuide, wantRecord: latestRecord},
		{name: "restore deleted guide", record: oldRecord, updated: []string{".wtf/" + guideChecksumFile}, wantGuide: workspaceGuide, wantRecord: latestRecord},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".wtf")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string][]byte{"AGENTS.md": tc.guide, guideChecksumFile: tc.record} {
				if data != nil {
					if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := New(root, t.TempDir(), nil).Setup(t.Context(), []byte("{}\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Updated) != len(tc.updated) || len(tc.updated) > 0 && !reflect.DeepEqual(result.Updated, tc.updated) {
				t.Fatalf("updates: %+v", result)
			}
			if tc.available != (len(result.UpdateAvailable) == 1) {
				t.Fatalf("update availability: %+v", result)
			}
			for name, expected := range map[string][]byte{"AGENTS.md": tc.wantGuide, guideChecksumFile: tc.wantRecord} {
				actual, err := os.ReadFile(filepath.Join(dir, name))
				if expected == nil {
					if !os.IsNotExist(err) {
						t.Fatalf("fabricated checksum: %q %v", actual, err)
					}
				} else if err != nil || !bytes.Equal(actual, expected) {
					t.Fatalf("%s: unexpected contents, error=%v", name, err)
				}
			}
		})
	}
}

func TestSetupGuideRejectsInvalidChecksumWithoutChangingGuide(t *testing.T) {
	for _, record := range []string{"", "not-a-checksum\n", "abcd\n"} {
		t.Run(record, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".wtf")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			guide := []byte("# Keep these edits\n")
			if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), guide, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, guideChecksumFile), []byte(record), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := New(root, t.TempDir(), nil).Setup(t.Context(), []byte("{}\n")); ErrorCode(err) != "invalid_setup_state" {
				t.Fatalf("invalid checksum accepted: %v", err)
			}
			actual, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if err != nil || !bytes.Equal(actual, guide) {
				t.Fatal("invalid checksum changed guide")
			}
		})
	}
}

func TestSetupGuideConcurrentRefreshAndCancelledWait(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".wtf")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := []byte("# Earlier template\n")
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, guideChecksumFile), []byte(guideChecksum(old)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock, err := lock(t.Context(), filepath.Join(dir, ".setup.lock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	var result SetupResult
	if err := refreshWorkspaceGuide(ctx, dir, workspaceGuide, &result); err != context.DeadlineExceeded {
		unlock()
		t.Fatalf("cancelled lock wait: %v", err)
	}
	unlock()
	actual, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil || !bytes.Equal(actual, old) {
		t.Fatal("cancelled wait changed guide")
	}
	results := make(chan SetupResult, 8)
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result SetupResult
			err := refreshWorkspaceGuide(t.Context(), dir, workspaceGuide, &result)
			results <- result
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	updates := 0
	for result := range results {
		updates += len(result.Updated)
		if len(result.UpdateAvailable) != 0 {
			t.Fatalf("spurious update conflict: %+v", result)
		}
	}
	if updates != 2 {
		t.Fatalf("guide/checksum replaced more than once: %d updates", updates)
	}
}

func TestSetupInheritedGuideCanRefreshIndependently(t *testing.T) {
	source, child := t.TempDir(), t.TempDir()
	if _, err := New(source, t.TempDir(), nil).Setup(t.Context(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	old := []byte("# Old template inherited from another workspace\n")
	if err := os.WriteFile(filepath.Join(source, ".wtf/AGENTS.md"), old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".wtf", guideChecksumFile), []byte(guideChecksum(old)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := inheritWorkspaceConfig(t.Context(), source, child); err != nil {
		t.Fatal(err)
	}
	result, err := New(child, t.TempDir(), nil).Setup(t.Context(), []byte("{}\n"))
	if err != nil || len(result.Updated) != 2 || len(result.UpdateAvailable) != 0 {
		t.Fatalf("inherited guide refresh: %+v %v", result, err)
	}
	actual, err := os.ReadFile(filepath.Join(source, ".wtf/AGENTS.md"))
	if err != nil || !bytes.Equal(actual, old) {
		t.Fatal("child refresh changed source guide")
	}
	if _, err := os.Stat(filepath.Join(child, ".wtf/.setup.lock")); err != nil {
		t.Fatal(err)
	}
}
