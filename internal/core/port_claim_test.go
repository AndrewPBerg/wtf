package core

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func portRegistry(t *testing.T, count int) (Repo, []Workspace) {
	t.Helper()
	r := Repo{Store: t.TempDir()}
	s := State{Version: 1, RepositoryID: "port-test", Workspaces: map[string]Workspace{}, Claims: map[string]Claim{}}
	owners := make([]Workspace, count)
	for i := range owners {
		owners[i] = Workspace{ID: fmt.Sprintf("owner-%d", i), Status: "ready"}
		s.Workspaces[owners[i].ID] = owners[i]
	}
	if err := writeAtomic(filepath.Join(r.Store, "state.json"), s); err != nil {
		t.Fatal(err)
	}
	return r, owners
}

func TestPortClaimContentionAndPersistence(t *testing.T) {
	r, owners := portRegistry(t, 32)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for round := range 8 {
		start := make(chan struct{})
		results := make(chan error, len(owners))
		var wg sync.WaitGroup
		for _, owner := range owners {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := r.Claim(ctx, owner, "port", "web", "127.0.0.1:3000")
				results <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if ErrorCode(err) != "claim_conflict" {
				t.Fatal(err)
			}
		}
		s, err := r.Load()
		if err != nil || success != 1 || len(s.Claims) != 1 {
			t.Fatalf("round %d: success=%d claims=%d error=%v", round, success, len(s.Claims), err)
		}
		for _, claim := range s.Claims {
			owner := s.Workspaces[claim.Owner]
			// Repeated intent is idempotent; another owner cannot release it.
			if _, err := r.Claim(ctx, owner, claim.Kind, claim.Name, claim.Value); err != nil {
				t.Fatal(err)
			}
			for _, other := range owners {
				if other.ID != owner.ID {
					if err := r.Release(ctx, other, "port", "web"); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			if _, err := r.Claim(ctx, owner, "port", "web", "127.0.0.1:3001"); ErrorCode(err) != "claim_conflict" {
				t.Fatalf("replacement accepted: %v", err)
			}
			if err := r.Release(ctx, owner, "port", "web"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Distinct concurrent writes must all survive the read/modify/write cycle.
	results := make(chan error, len(owners))
	for i, owner := range owners {
		go func() {
			_, err := r.Claim(ctx, owner, "port", "web", fmt.Sprintf("127.0.0.1:%d", 3100+i))
			results <- err
		}()
	}
	for range owners {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	s, err := r.Load()
	if err != nil || len(s.Claims) != len(owners) {
		t.Fatalf("lost claims: %d %v", len(s.Claims), err)
	}
}

func TestPortClaimAcrossProcesses(t *testing.T) {
	if store := os.Getenv("WTF_PORT_TEST_STORE"); store != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for {
			if _, err := os.Stat(filepath.Join(store, "start")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(5 * time.Millisecond):
			}
		}
		r := Repo{Store: store}
		owner := Workspace{ID: os.Getenv("WTF_PORT_TEST_OWNER")}
		for i := range 8 {
			_, err := r.Claim(ctx, owner, "port", fmt.Sprintf("web-%d", i), fmt.Sprintf("127.0.0.1:%d", 3200+i))
			if err != nil && ErrorCode(err) != "claim_conflict" {
				t.Fatal(err)
			}
		}
		return
	}
	r, owners := portRegistry(t, 8)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	commands := make([]*exec.Cmd, len(owners))
	outputs := make([]bytes.Buffer, len(owners))
	for i, owner := range owners {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestPortClaimAcrossProcesses$")
		cmd.Env = append(os.Environ(), "WTF_PORT_TEST_STORE="+r.Store, "WTF_PORT_TEST_OWNER="+owner.ID, "GOMAXPROCS=1")
		cmd.Stdout, cmd.Stderr = &outputs[i], &outputs[i]
		commands[i] = cmd
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.Store, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child %d: %v %s", i, err, &outputs[i])
		}
	}
	s, err := r.Load()
	if err != nil || len(s.Claims) != 8 {
		t.Fatalf("process contention: claims=%d error=%v", len(s.Claims), err)
	}
	values := map[string]bool{}
	for _, claim := range s.Claims {
		if values[claim.Value] {
			t.Fatalf("duplicate ownership: %+v", claim)
		}
		values[claim.Value] = true
	}
}

func TestPortClaimRepositoryScopeAndAliases(t *testing.T) {
	r, owners := portRegistry(t, 2)
	ctx := context.Background()
	const value = "127.0.0.1:3300"
	for _, name := range []string{"web", "alias"} {
		if _, err := r.Claim(ctx, owners[0], "port", name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Release(ctx, owners[0], "port", "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim(ctx, owners[1], "port", "different-name", value); ErrorCode(err) != "claim_conflict" {
		t.Fatalf("alias did not retain ownership: %v", err)
	}
	otherRepo, otherOwners := portRegistry(t, 1)
	if _, err := otherRepo.Claim(ctx, otherOwners[0], "port", "web", value); err != nil {
		t.Fatalf("independent repository unexpectedly shared claims: %v", err)
	}
	if err := r.Release(ctx, owners[0], "port", "alias"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim(ctx, owners[1], "port", "web", value); err != nil {
		t.Fatalf("released port could not transfer ownership: %v", err)
	}
	if err := r.Edit(ctx, func(s *State) error {
		w := s.Workspaces[owners[0].ID]
		w.Status = "removed"
		s.Workspaces[w.ID] = w
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim(ctx, owners[0], "port", "web", "127.0.0.1:3301"); ErrorCode(err) != "workspace_missing" {
		t.Fatalf("removed owner claimed a port: %v", err)
	}
}

func TestLuaPortClaimRetryAndRelease(t *testing.T) {
	a, r, w := fixture(t)
	listeners := make([]net.Listener, 3)
	ports := make([]int, 3)
	for i := range listeners {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		listeners[i] = listener
		ports[i] = listener.Addr().(*net.TCPAddr).Port
	}
	_ = listeners[1].Close()
	_ = listeners[2].Close()
	other := Workspace{ID: "other", Status: "ready"}
	if err := r.Edit(context.Background(), func(s *State) error { s.Workspaces[other.ID] = other; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Claim(context.Background(), other, "port", "web", "127.0.0.1:"+strconv.Itoa(ports[1])); err != nil {
		t.Fatal(err)
	}
	packageFiles(t, w.Path, map[string]string{"init.lua": fmt.Sprintf(`
local resources = require("wtf.resources")
return require("wtf").setup { hooks = { allocate = function(event)
  assert(resources.listening(%d))
  local rejected, code = resources.claim_port("web", %d)
  assert(rejected == nil and code == "claim_conflict")
  local selected
  for _, port in ipairs({%d, %d, %d}) do
    if not resources.listening(port) then
      local value, err = resources.claim_port("web", port)
      if value then selected = value; break end
      assert(err == "claim_conflict")
    end
  end
  assert(selected == %d)
  assert(resources.claim_port("web", selected) == selected)
  local claims = resources.list()
  assert(#claims == 1 and claims[1].value == "127.0.0.1:" .. selected)
  resources.release("port", "web")
  assert(#resources.list() == 0)
  assert(resources.claim_port("web", selected) == selected)
end } }`, ports[0], ports[1], ports[0], ports[1], ports[2], ports[2])})
	if _, err := a.Hook(context.Background(), "allocate"); err != nil {
		t.Fatal(err)
	}
	// The registry claim did not bind a socket: an unrelated service can take it.
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", ports[2]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := r.Release(context.Background(), w, "port", "web"); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("release stopped the listener: %v", err)
	}
	_ = conn.Close()
}
