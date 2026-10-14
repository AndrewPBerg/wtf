package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupCancellationBeforeMutation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(root, t.TempDir(), nil).Setup(ctx, []byte("{}\n"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".wtf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled setup mutated directory: %v", err)
	}
}

func TestSetupTeardownRequiresConfirmationAndPreservesExternalFiles(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	a := New(root, t.TempDir(), nil)
	if _, err := a.Setup(context.Background(), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "source.txt"), filepath.Join(outside, "secret.txt"), filepath.Join(root, ".wtf", "tasks", "custom.lua")} {
		if err := os.WriteFile(path, []byte("preserve outside, remove inside"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, ".wtf", "hooks", "linked")); err != nil {
		t.Fatal(err)
	}
	preview, err := a.TeardownSetup(context.Background(), false)
	if err != nil || preview.Status != "confirmation_required" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".wtf", "config.toml")); err != nil {
		t.Fatalf("unconfirmed teardown removed config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.TeardownSetup(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled teardown: %v", err)
	}
	result, err := a.TeardownSetup(context.Background(), true)
	if err != nil || result.Status != "removed" {
		t.Fatalf("confirmed teardown: %+v %v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".wtf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("configuration remains: %v", err)
	}
	for _, path := range []string{filepath.Join(root, "source.txt"), filepath.Join(outside, "secret.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("teardown removed external file %s: %v", path, err)
		}
	}
	result, err = a.TeardownSetup(context.Background(), false)
	if err != nil || result.Status != "not_configured" {
		t.Fatalf("repeated teardown: %+v %v", result, err)
	}
}

func TestSetupTeardownRejectsSymlinkRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".wtf")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, t.TempDir(), nil).TeardownSetup(context.Background(), true); ErrorCode(err) != "invalid_setup_path" {
		t.Fatalf("accepted symlinked .wtf: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".wtf")); err != nil {
		t.Fatalf("symlink removed: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
}
