package identity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUUID(t *testing.T) {
	id, err := NewID()
	require.NoError(t, err)
	require.NoError(t, ValidateID(id))
	require.Equal(t, strings.ToLower(id), id)
	for _, bad := range []string{"", "550E8400-E29B-41D4-A716-446655440000", "550e8400-e29b-61d4-a716-446655440000", "550e8400-e29b-41d4-c716-446655440000", "not-a-uuid"} {
		require.Error(t, ValidateID(bad))
	}
}

func TestNameValidation(t *testing.T) {
	for _, name := range []string{"wtf/default", "alita-core/feature/auth-refresh"} {
		require.NoError(t, ValidateName(name))
	}
	for _, name := range []string{"", "Wtf/main", "one", "/wtf/main", "wtf//main", "wtf/../main", "wtf/a\\b", "wtf/a\nb"} {
		require.Error(t, ValidateName(name))
	}
}

func TestActiveRepositoryAndBackendAreRequired(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	_, err = s.CreateWorkspace(repo.ID, "repo/main", string(Git), "main", filepath.Join(t.TempDir(), "w"))
	require.NoError(t, err)
	_, err = s.CreateWorkspace(repo.ID, "repo/other", "svn", "other", filepath.Join(t.TempDir(), "w2"))
	require.Error(t, err)
	state, err := s.Load()
	require.NoError(t, err)
	state.Repositories[0].LifecycleState = Removed
	require.Error(t, state.Validate())
}

func TestCleanupFailureRequiresExplicitFinalize(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	w, err := s.CreateWorkspace(repo.ID, "repo/main", string(Git), "main", filepath.Join(t.TempDir(), "w"))
	require.NoError(t, err)
	_, err = s.ActivateWorkspace(w.ID)
	require.NoError(t, err)
	failed, err := s.MarkCleanupFailed(w.ID)
	require.NoError(t, err)
	require.NotEmpty(t, failed.CleanupFailedAt)
	require.Empty(t, failed.RemovedAt)
	_, err = s.RemoveWorkspace(w.ID)
	require.Error(t, err)
	removed, err := s.FinalizeCleanup(w.ID)
	require.NoError(t, err)
	require.Equal(t, Removed, removed.LifecycleState)
	require.NotEmpty(t, removed.RemovedAt)
	require.Equal(t, failed.CleanupFailedAt, removed.CleanupFailedAt)
}

func TestSymlinkAliasesCanonicalizePendingPaths(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realRoot, 0o755))
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(realRoot, alias))
	first, err := s.CreateWorkspace(repo.ID, "repo/one", string(Git), "one", filepath.Join(realRoot, "new"))
	require.NoError(t, err)
	_, err = s.CreateWorkspace(repo.ID, "repo/two", string(Git), "two", filepath.Join(alias, "new"))
	require.Error(t, err)
	require.Equal(t, filepath.Join(realRoot, "new"), first.Path)
}

func TestLifecycleTombstoneAndReuse(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	first, err := s.CreateWorkspace(repo.ID, "repo/main", "jj", "repo/main", "/tmp/main")
	require.NoError(t, err)
	_, err = s.ActivateWorkspace(first.ID)
	require.NoError(t, err)
	_, err = s.CreateWorkspace(repo.ID, "repo/main", "jj", "repo/main", "/tmp/other")
	require.Error(t, err) // pending claims are global
	_, err = s.RemoveWorkspace(first.ID)
	require.NoError(t, err)
	second, err := s.CreateWorkspace(repo.ID, "repo/main", "jj", "repo/main", "/tmp/main")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	loaded, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, Removed, loaded.Workspaces[0].LifecycleState)
	require.NoError(t, loaded.Validate())
	old, err := s.LookupWorkspace(first.ID)
	require.NoError(t, err)
	require.Equal(t, Removed, old.LifecycleState)
	byName, err := s.LookupWorkspace("repo/main")
	require.NoError(t, err)
	require.Equal(t, second.ID, byName.ID)
}

func TestClaimsAreAtomic(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.CreateWorkspace(repo.ID, "repo/same", "git", "branch", filepath.Join(t.TempDir(), "w"))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
}

func TestSubprocessConcurrencyAndCrashRecovery(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	results := make(chan string, 2)
	barrier := filepath.Join(t.TempDir(), "release")
	for i := 0; i < 2; i++ {
		path := filepath.Join(t.TempDir(), "workspace")
		go func() {
			cmd := exec.Command(os.Args[0], "-test.run=TestIdentitySubprocessHelper", "-test.v=false", s.home, repo.ID, path)
			cmd.Env = append(os.Environ(), "WTF_IDENTITY_HELPER=claim", "WTF_IDENTITY_BARRIER="+barrier)
			out, runErr := cmd.CombinedOutput()
			if runErr != nil {
				t.Logf("subprocess failed: %v: %s", runErr, out)
				results <- "failure"
				return
			}
			results <- strings.TrimSpace(string(out))
		}()
	}
	ready := []string{}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		ready, _ = filepath.Glob(barrier + ".ready.*")
		if len(ready) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Len(t, ready, 2)
	require.NoError(t, os.WriteFile(barrier, nil, 0o600))
	claims := 0
	collisions := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if strings.HasPrefix(result, "claimed") {
			claims++
		}
		if strings.HasPrefix(result, "collision") {
			collisions++
		}
	}
	require.Equal(t, 1, claims)
	require.Equal(t, 1, collisions)
	final, err := s.Load()
	require.NoError(t, err)
	claimed := 0
	for _, w := range final.Workspaces {
		if w.Name == "repo/concurrent" {
			claimed++
			require.Equal(t, Pending, w.LifecycleState)
		}
	}
	require.Equal(t, 1, claimed)

	cmd := exec.Command(os.Args[0], "-test.run=TestIdentitySubprocessHelper", "-test.v=false", s.home)
	cmd.Env = append(os.Environ(), "WTF_IDENTITY_HELPER=crash-lock")
	require.Error(t, cmd.Run())
	_, err = s.CreateRepository(filepath.Join(t.TempDir(), "after-crash"))
	require.NoError(t, err)
}

func TestIdentitySubprocessHelper(t *testing.T) {
	mode := os.Getenv("WTF_IDENTITY_HELPER")
	if mode == "" {
		return
	}
	home := os.Args[len(os.Args)-1]
	if mode == "claim" {
		home = os.Args[len(os.Args)-3]
	}
	s, err := NewStore(home)
	require.NoError(t, err)
	if mode == "claim" {
		if barrier := os.Getenv("WTF_IDENTITY_BARRIER"); barrier != "" {
			_ = os.WriteFile(barrier+".ready."+fmt.Sprint(os.Getpid()), nil, 0o600)
			for {
				if _, e := os.Stat(barrier); e == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		_, err = s.CreateWorkspace(os.Args[len(os.Args)-2], "repo/concurrent", "git", "branch", os.Args[len(os.Args)-1])
		if err != nil {
			_, _ = os.Stdout.Write([]byte("collision"))
			return
		}
		_, _ = os.Stdout.Write([]byte("claimed"))
		return
	}
	unlock, err := lockFile(s.lockPath)
	require.NoError(t, err)
	_ = unlock
	os.Exit(17)
}

func TestCorruptStateFailsClosed(t *testing.T) {
	s := mustStore(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(mustPath(s)), 0o700))
	original := []byte(`{"version":999}`)
	require.NoError(t, os.WriteFile(mustPath(s), original, 0o600))
	_, err := s.Load()
	require.Error(t, err)
	_, err = s.CreateRepository("x")
	require.Error(t, err)
	got, err := os.ReadFile(mustPath(s))
	require.NoError(t, err)
	require.Equal(t, original, got)
}

func TestUnsupportedAndUnknownFieldsFail(t *testing.T) {
	s := mustStore(t)
	require.NoError(t, os.MkdirAll(s.home, 0o700))
	require.NoError(t, os.WriteFile(mustPath(s), []byte(`{"version":1,"repositories":[],"workspaces":[],"extra":true}`), 0o600))
	_, err := s.Load()
	require.Error(t, err)
}

func mustStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	return s
}
func mustPath(s *Store) string { p, _ := s.Paths(); return p }

func TestConcurrentMarkerWritersAreCompareAndSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "marker")
	ids := []string{}
	for i := 0; i < 2; i++ {
		id, err := NewID()
		require.NoError(t, err)
		ids = append(ids, id)
	}
	results := make(chan error, len(ids))
	for _, id := range ids {
		go func() { results <- WriteRepositoryID(dir, id) }()
	}
	var successes int
	for range ids {
		if err := <-results; err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	got, err := ReadRepositoryID(dir)
	require.NoError(t, err)
	require.Contains(t, ids, got)
}

func TestStateDirectoryMustBeAbsolutePhysicalAndLocal(t *testing.T) {
	id, err := NewID()
	require.NoError(t, err)
	for _, dir := range []string{"relative", "", "https://example.invalid/state", "bad\x00dir"} {
		require.Error(t, WriteRepositoryID(dir, id))
	}
	realDir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(realDir, alias))
	require.Error(t, WriteRepositoryID(alias, id))
}

func TestRepeatedAdoptionReturnsExistingWorkspace(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "existing")
	first, err := s.AdoptWorkspace(repo.ID, "repo/existing", "jj", "repo/existing", path)
	require.NoError(t, err)
	second, err := s.AdoptWorkspace(repo.ID, "repo/existing", "jj", "repo/existing", path)
	require.NoError(t, err)
	require.Equal(t, Adopted, second.Status)
	require.Equal(t, first.Workspace, second.Workspace)
	state, err := s.Load()
	require.NoError(t, err)
	require.Len(t, state.Workspaces, 1)
}

func TestExactPendingAdoptionRequiresRepairWithoutMutation(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "pending")
	pending, err := s.CreateWorkspace(repo.ID, "repo/pending", string(JJ), "repo/pending", path)
	require.NoError(t, err)
	statePath, _ := s.Paths()
	before, err := os.ReadFile(statePath)
	require.NoError(t, err)

	result, err := s.AdoptWorkspace(repo.ID, pending.Name, pending.Backend, pending.NativeName, pending.Path)
	require.NoError(t, err)
	require.Equal(t, RepairRequired, result.Status)
	require.Equal(t, pending, result.Workspace)
	after, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestExactCleanupFailedAdoptionRequiresRepairWithoutMutation(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "failed")
	pending, err := s.CreateWorkspace(repo.ID, "repo/failed", string(JJ), "repo/failed", path)
	require.NoError(t, err)
	failed, err := s.MarkCleanupFailed(pending.ID)
	require.NoError(t, err)
	statePath, _ := s.Paths()
	before, err := os.ReadFile(statePath)
	require.NoError(t, err)

	result, err := s.AdoptWorkspace(repo.ID, failed.Name, failed.Backend, failed.NativeName, failed.Path)
	require.NoError(t, err)
	require.Equal(t, RepairRequired, result.Status)
	require.Equal(t, failed, result.Workspace)
	after, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestRepositoryMarkerReconciliationAndCorruption(t *testing.T) {
	s := mustStore(t)
	locator := filepath.Join(t.TempDir(), "repo")
	markerDir := filepath.Join(t.TempDir(), ".jj", "repo", "wtf")
	r, err := s.EnsureRepository(locator, markerDir)
	require.NoError(t, err)
	got, err := ReadRepositoryID(markerDir)
	require.NoError(t, err)
	require.Equal(t, r.ID, got)
	again, err := s.EnsureRepository(locator, markerDir)
	require.NoError(t, err)
	require.Equal(t, r.ID, again.ID)
	require.NoError(t, os.WriteFile(RepositoryMarkerPath(markerDir), []byte("bad\n"), 0o600))
	_, err = s.EnsureRepository(locator, markerDir)
	require.Error(t, err)
}

func TestMarkerCanRepairMissingGlobalState(t *testing.T) {
	s := mustStore(t)
	locator := filepath.Join(t.TempDir(), "repo")
	dir := filepath.Join(t.TempDir(), "git-wtf")
	id, _ := NewID()
	require.NoError(t, WriteRepositoryID(dir, id))
	r, err := s.EnsureRepository(locator, dir)
	require.NoError(t, err)
	require.Equal(t, id, r.ID)
}

func TestAdoptionValidationAndQueries(t *testing.T) {
	s := mustStore(t)
	repo, err := s.CreateRepository(filepath.Join(t.TempDir(), "repo"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "existing")
	result, err := s.AdoptWorkspace(repo.ID, "repo/existing", "jj", "repo/existing", path)
	require.NoError(t, err)
	require.Equal(t, Adopted, result.Status)
	byID, err := s.LookupWorkspace(result.Workspace.ID)
	require.NoError(t, err)
	require.Equal(t, result.Workspace.ID, byID.ID)
	byPath, err := s.FindWorkspace(path)
	require.NoError(t, err)
	require.Equal(t, result.Workspace.ID, byPath.ID)
	legacy, err := s.AdoptWorkspace(repo.ID, "default", "jj", "default", filepath.Join(t.TempDir(), "legacy"))
	require.NoError(t, err)
	require.Equal(t, RenameRequired, legacy.Status)
	state, err := s.Load()
	require.NoError(t, err)
	require.Len(t, state.Workspaces, 1)
	collision, err := s.AdoptWorkspace(repo.ID, "repo/existing", "git", "branch", filepath.Join(t.TempDir(), "other"))
	require.NoError(t, err)
	require.Equal(t, RenameRequired, collision.Status)
}
