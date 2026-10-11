//go:build linux

package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the real embedded adapter and RPC scheduler through a local transport.
// Mock only process transport; real request IDs, parsing and scheduling run unchanged.
// The fixture has no credentials, network transport, or inference capability.
func reviewBridgeFixture(t *testing.T, mode string) ([]ReviewLookup, string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the embedded adapter protocol test")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	// PATH may expose a launcher; the fixture's shebang needs the runtime itself.
	runtime, err := exec.Command(node, "-p", "process.execPath").Output()
	if err != nil {
		t.Fatal(err)
	}
	node = strings.TrimSpace(string(runtime))
	directory := t.TempDir()
	source := "#!" + node + "\n" + `
import { appendFileSync, readFileSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { PassThrough } from 'node:stream';
import { EventEmitter } from 'node:events';
import { resolve } from 'node:path';
import { realpathSync } from 'node:fs';
import { homedir } from 'node:os';
const log = value => appendFileSync(process.env.WTF_REVIEW_RPC_LOG, JSON.stringify(value)+'\n');
const candidate = [{name:'linear',enabled:true,auth_status:'o_auth',transport:{type:'streamable_http',url:'https://mcp.linear.app/mcp'}}];
const spawnSync = (_,args) => ({status:0,stdout:args[0]==='which'?'fixture-codex':JSON.stringify(candidate)});
const spawn = () => {
  const child = new EventEmitter();
  child.pid = process.pid;
  child.exitCode = null;
  child.stdout = new PassThrough();
  child.stderr = new PassThrough();
  child.stdin = new PassThrough();
  child.kill = () => {
    child.exitCode = 0;
    queueMicrotask(()=>child.emit('exit',0));
  };
  log({event:'start',pid:process.pid});
  let threads=0,active=0;
  const verified = new Set();
  const reply = (id,result) => child.stdout.write(JSON.stringify({id,result})+'\n');
  const content = value => ({content:[{type:'text',text:JSON.stringify(value)}]});
  createInterface({input:child.stdin}).on('line',line=>{
    const request=JSON.parse(line);
    if (!request.method) return;
    log({event:request.method});
    if (request.method==='initialize') return reply(request.id,{});
    if (request.method==='thread/start') return reply(request.id,{thread:{id:'thread-'+(++threads),ephemeral:true}});
    if (request.method==='initialized') return;
    if (request.method!=='mcpServer/tool/call') throw new Error('Unexpected method');
    const {threadId,tool,arguments:args}=request.params;
    log({event:tool,threadId});
    if (tool==='get_workspace') {
      verified.add(threadId);
      const wrong=process.env.WTF_REVIEW_RPC_MODE==='wrong_second' && threadId==='thread-2';
      const workspaceURL=process.env.WTF_REVIEW_RPC_MODE==='wrong_workspace_url'?'https://evil.example/cascadinglabs':'https://linear.app/cascadinglabs';
      return reply(request.id,content({id:wrong?'other-id':'workspace-id',url:workspaceURL,name:'Fixture'}));
    }
    if (!verified.has(threadId)) throw new Error('Lookup before verification');
    const number=Number(args.urlOrId.split('/').at(-1));
    if (process.env.WTF_REVIEW_RPC_MODE==='hang') return;
    if (process.env.WTF_REVIEW_RPC_MODE==='throttle' && number===1) return reply(request.id,{isError:true,content:[{type:'text',text:'RATELIMITED: retry after 60 seconds'}]});
    active++;
    log({event:'active',active});
    setTimeout(()=>{
      active--;
      if (process.env.WTF_REVIEW_RPC_MODE==='missing' && number===2) return reply(request.id,{isError:true,content:[{type:'text',text:'Diff not found'}]});
      const workspace=process.env.WTF_REVIEW_RPC_MODE==='wrong_url'?'alitahealth':'cascadinglabs';
      reply(request.id,content({url:args.urlOrId,appUrl:'https://linear.app/'+workspace+'/review/pr-'+number}));
    },10+(number%4)*10);
  });
  return child;
};
const filename = process.argv[2];
const source = readFileSync(filename,'utf8').replace(/^#![^\n]*\n/,'').replace(/^import .*;\n/gm,'');
const bridgeProcess = {argv:[process.argv[0],filename,...process.argv.slice(3)],stdout:process.stdout,exitCode:0,exit:code=>{throw new Error('Unexpected early exit '+code)}};
const AsyncFunction = Object.getPrototypeOf(async function(){}).constructor;
await new AsyncFunction('spawn','spawnSync','resolve','createInterface','homedir','realpathSync','readFileSync','process',source)(spawn,spawnSync,resolve,createInterface,homedir,realpathSync,readFileSync,bridgeProcess);
process.exitCode=bridgeProcess.exitCode;
`
	if err = os.WriteFile(filepath.Join(directory, "node"), []byte(source), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(directory, "rpc.jsonl")
	t.Setenv("PATH", directory)
	t.Setenv("WTF_REVIEW_RPC_LOG", log)
	t.Setenv("WTF_REVIEW_RPC_MODE", mode)
	lookups := make([]ReviewLookup, 32)
	for i := range lookups {
		lookups[i] = ReviewLookup{Root: directory, URL: "https://github.com/owner/repo/pull/" + strconv.Itoa(i+1)}
	}
	return lookups, log
}

func bridgeEvents(t *testing.T, path string) []struct {
	Event       string
	Active, PID int
} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	events := []struct {
		Event       string
		Active, PID int
	}{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Event       string
			Active, PID int
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestNativeReviewBatchProtocolUsesOneProcessTwoVerifiedContextsAndEightCalls(t *testing.T) {
	lookups, log := reviewBridgeFixture(t, "missing")
	results := (NativeReviewClient{}).ResolveBatch(t.Context(), lookups)
	for i, result := range results {
		if i == 1 {
			if result.Code != "review_not_found" || result.Provider != "github" || result.URL != lookups[i].URL {
				t.Fatal("missing review lost its exact fallback")
			}
		} else if result.Code != "review_found" || result.URL != "https://linear.app/cascadinglabs/review/pr-"+strconv.Itoa(i+1) {
			t.Fatalf("out-of-order response attached to wrong PR at %d: %+v", i, result)
		}
	}
	starts, verifications, calls, peak := 0, 0, 0, 0
	for _, event := range bridgeEvents(t, log) {
		switch event.Event {
		case "start":
			starts++
		case "get_workspace":
			verifications++
		case "get_diff":
			calls++
		case "active":
			peak = max(peak, event.Active)
		}
		if strings.HasPrefix(event.Event, "turn/") {
			t.Fatal("provider read started a model turn")
		}
	}
	if starts != 1 || verifications != 2 || calls != 32 || peak != 8 {
		t.Fatalf("starts=%d verification=%d calls=%d peak=%d", starts, verifications, calls, peak)
	}
}

func TestNativeReviewBatchProtocolStopsAfterThrottlingAndRejectsWrongWorkspace(t *testing.T) {
	for _, mode := range []string{"throttle", "wrong_second", "wrong_workspace_url", "wrong_url"} {
		t.Run(mode, func(t *testing.T) {
			lookups, log := reviewBridgeFixture(t, mode)
			results := (NativeReviewClient{}).ResolveBatch(t.Context(), lookups)
			for i, result := range results {
				if result.Code == "review_not_found" || (mode != "throttle" && (result.Provider != "github" || result.URL != lookups[i].URL)) {
					t.Fatal("failure became a verified or missing association")
				}
			}
			calls := 0
			for _, event := range bridgeEvents(t, log) {
				if event.Event == "get_diff" {
					calls++
				}
			}
			if (mode == "wrong_second" || mode == "wrong_workspace_url") && calls != 0 {
				t.Fatal("lookup ran before both contexts were verified")
			}
			if mode == "throttle" && (calls > 8 || results[0].Code != "linear_throttled") {
				t.Fatal("throttling did not stop queued work")
			}
		})
	}
}

func TestNativeReviewBatchProtocolCancellationCleansUpBridge(t *testing.T) {
	lookups, log := reviewBridgeFixture(t, "hang")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			data, _ := os.ReadFile(log)
			if strings.Contains(string(data), `"event":"get_diff"`) {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	results := (NativeReviewClient{}).ResolveBatch(ctx, lookups)
	for _, result := range results {
		if result.Code != "review_lookup_cancelled" {
			t.Fatal("cancelled work lost its cancellation code")
		}
	}
	for _, event := range bridgeEvents(t, log) {
		if event.Event == "start" {
			for attempt := 0; attempt < 50; attempt++ {
				if syscall.Kill(event.PID, 0) != nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("cancelled provider left its bridge process alive")
		}
	}
}
