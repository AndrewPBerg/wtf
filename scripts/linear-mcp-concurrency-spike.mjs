#!/usr/bin/env node
// Read-only benchmark: existing Codex OAuth, no model turns, no raw diagnostics.
// Run from the WTF repository; --help describes the bounded layouts.
import { spawnSync } from 'node:child_process';
import { readFileSync, readdirSync, realpathSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { homedir } from 'node:os';
import { createHash } from 'node:crypto';

const options = new Map();
for (let i = 2; i < process.argv.length; i += 2) {
  if (process.argv[i] === '--help') {
    console.log('node scripts/linear-mcp-concurrency-spike.mjs --repo PATH --manifest JSON --output JSON --mode pool|fresh|list --processes 1 --threads 1 --concurrency 1 --jobs 32 --rounds 1 [--expect-workspace alitahealth]');
    process.exit(0);
  }
  const key = process.argv[i], value = process.argv[i + 1];
  if (!['--repo', '--manifest', '--output', '--mode', '--processes', '--threads', '--concurrency', '--jobs', '--rounds', '--expect-workspace', '--startup-timeout-ms', '--cancel-after-ms'].includes(key) || !value) throw new Error('invalid arguments');
  options.set(key, value);
}
const repo = realpathSync(resolve(options.get('--repo') ?? process.cwd()));
const mode = options.get('--mode') ?? 'pool';
const integer = (key, fallback, maximum) => {
  const value = Number(options.get(key) ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > maximum) throw new Error(`invalid ${key}`);
  return value;
};
const processes = integer('--processes', 1, 2); // User-selected workstation ceiling.
const threads = integer('--threads', 1, 8);
const concurrency = integer('--concurrency', 1, 50);
const jobs = integer('--jobs', 32, 100);
const rounds = integer('--rounds', 1, 5);
if (!['pool', 'fresh', 'list'].includes(mode) || processes * threads * concurrency > 50 || (mode === 'fresh' && (threads !== 1 || concurrency !== 1)) || (mode === 'list' && (processes !== 1 || threads !== 1 || rounds !== 1))) throw new Error('unsupported layout');
const manifest = JSON.parse(readFileSync(options.get('--manifest'), 'utf8'));
if (!Array.isArray(manifest.urls) || manifest.urls.length === 0 || manifest.urls.some(url => !/^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pull\/[1-9][0-9]*$/.test(url))) throw new Error('invalid manifest');
const urls = Array.from({ length: jobs }, (_, index) => manifest.urls[index % manifest.urls.length]);
const timeout = 5000; // Match the production bridge's per-RPC deadline.
const startupTimeout = integer('--startup-timeout-ms', 5000, 10000);
const bridge = readFileSync('internal/core/review_bridge.mjs', 'utf8');
const helpers = bridge.slice(bridge.indexOf('function fault(code)'), bridge.indexOf('\nlet rpc;'));
if (!helpers.includes('class RPC') || !helpers.includes('function parseTool')) throw new Error('bridge helper contract changed');
const helperModule = "import { spawn } from 'node:child_process';\nimport { createInterface } from 'node:readline';\n" + helpers + '\nexport {RPC, parseTool, fault};';
const { RPC, parseTool, fault } = await import('data:text/javascript;base64,' + Buffer.from(helperModule).toString('base64'));
const startedAt = performance.now();
const allRPCs = new Set();
const managedChildren = [];
let stopping = null;
let active = 0, peakActive = 0;
let startedProcesses = 0;
let toolCalls = 0;
const sessions = [];
const outcomes = [];

function machine() {
  const fields = {};
  for (const line of readFileSync('/proc/meminfo', 'utf8').split('\n')) {
    const match = /^(MemAvailable|SwapFree):\s+(\d+)/.exec(line);
    if (match) fields[match[1]] = Number(match[2]);
  }
  return fields;
}
const initialMachine = machine();
if (initialMachine.MemAvailable < 2 * 1024 * 1024) throw new Error('insufficient memory headroom');
const resources = { peak_tree_rss_kib: 0, peak_tree_pss_kib: 0, peak_tree_private_kib: 0, pss_sampled: false, min_available_kib: initialMachine.MemAvailable, swap_used_delta_kib: 0, peak_tree_processes: 0 };
function stop(reason) {
  stopping ??= reason;
  for (const rpc of allRPCs) {
    try { rpc.process.kill('SIGTERM'); } catch {}
  }
}
function sampleResources() {
  const mem = machine();
  resources.min_available_kib = Math.min(resources.min_available_kib, mem.MemAvailable);
  resources.swap_used_delta_kib = Math.max(resources.swap_used_delta_kib, initialMachine.SwapFree - mem.SwapFree);
  let rss = 0, pss = 0, privateMemory = 0, count = 0;
  const tree = new Set([process.pid]);
  // Scan only this harness's descendants. Include Node + every Codex app-server.
  const records = [];
  for (const entry of readdirSync('/proc')) {
    if (!/^\d+$/.test(entry)) continue;
    try {
      const status = readFileSync(`/proc/${entry}/status`, 'utf8');
      records.push({ pid: Number(entry), parent: Number(/^PPid:\s+(\d+)/m.exec(status)?.[1]), rss: Number(/^VmRSS:\s+(\d+)/m.exec(status)?.[1] ?? 0) });
    } catch {}
  }
  for (let added = true; added;) {
    added = false;
    for (const row of records) if (tree.has(row.parent) && !tree.has(row.pid)) { tree.add(row.pid); added = true; }
  }
  for (const row of records) if (tree.has(row.pid)) {
    rss += row.rss;
    count++;
    try {
      const memory = readFileSync(`/proc/${row.pid}/smaps_rollup`, 'utf8');
      pss += Number(/^Pss:\s+(\d+)/m.exec(memory)?.[1] ?? 0);
      privateMemory += Number(/^Private_Clean:\s+(\d+)/m.exec(memory)?.[1] ?? 0) + Number(/^Private_Dirty:\s+(\d+)/m.exec(memory)?.[1] ?? 0);
      resources.pss_sampled = true;
    } catch {}
  }
  resources.peak_tree_rss_kib = Math.max(resources.peak_tree_rss_kib, rss);
  resources.peak_tree_processes = Math.max(resources.peak_tree_processes, count);
  resources.peak_tree_pss_kib = Math.max(resources.peak_tree_pss_kib, pss);
  resources.peak_tree_private_kib = Math.max(resources.peak_tree_private_kib, privateMemory);
  if (mem.MemAvailable < 2 * 1024 * 1024 || resources.swap_used_delta_kib > 256 * 1024 || pss > 1024 * 1024 || rss > 1280 * 1024) stop('resource_stop');
}
const sampler = setInterval(sampleResources, 200);
const deadline = setTimeout(() => stop('stage_deadline'), 90000);
const cancelTimer = options.has('--cancel-after-ms') ? setTimeout(() => stop('injected_cancellation'), integer('--cancel-after-ms', 1, 5000)) : null;
process.on('SIGTERM', () => stop('cancelled'));
process.on('SIGINT', () => stop('cancelled'));

const setupStarted = performance.now();
const under = root => repo === root || repo.startsWith(root + '/');
const profile = under(resolve(homedir(), 'Desktop/Work')) ? 'alita' : under(resolve(homedir(), 'Desktop/cl')) ? 'cascading' : null;
const locate = spawnSync('mise', ['which', 'codex'], { encoding: 'utf8', timeout: 5000 });
const binary = locate.status === 0 ? locate.stdout.trim() : 'codex';
const discovery = spawnSync(binary, [...(profile ? ['--profile', profile] : []), 'mcp', 'list', '--json'], { cwd: repo, encoding: 'utf8', timeout: 5000, maxBuffer: 2 * 1024 * 1024 });
if (discovery.status !== 0) throw new Error('codex configuration unavailable; diagnostics withheld');
const servers = JSON.parse(discovery.stdout);
const candidates = servers.filter(s => s.enabled && s.transport?.type === 'streamable_http' && s.transport.url === 'https://mcp.linear.app/mcp');
if (candidates.length !== 1) throw new Error('ambiguous or missing Linear connection');
const selected = candidates[0];
const expected = options.get('--expect-workspace') ?? ({ linear: 'cascadinglabs', 'linear-alita': 'alitahealth' })[selected.name];
if (!expected || !['o_auth', 'unknown'].includes(selected.auth_status) || selected.transport.bearer_token_env_var || Object.keys(selected.transport.http_headers ?? {}).length || Object.keys(selected.transport.env_http_headers ?? {}).length) throw new Error('unsupported account configuration');
const childArgs = [];
for (const server of servers) {
  if (!/^[A-Za-z0-9_-]+$/.test(server.name)) throw new Error('unsupported server identifier');
  if (server.name !== selected.name) childArgs.push('-c', `mcp_servers.${server.name}=${server.transport?.type === 'stdio' ? '{command="/usr/bin/false",args=[],enabled=false}' : '{url="https://mcp.linear.app/mcp",enabled=false}'}`);
}
childArgs.push('-c', `mcp_servers.${selected.name}={url="https://mcp.linear.app/mcp",enabled=true,enabled_tools=${JSON.stringify(['get_workspace', 'get_diff', ...(mode === 'list' ? ['list_diffs'] : [])])}}`, 'app-server', '--stdio');
const discoveryMS = performance.now() - setupStarted;

function reason(error) {
  return /^[a-zA-Z0-9_:./-]+$/.test(error?.code ?? '') ? error.code : 'spike_failure';
}
async function tool(rpc, thread, name, args, requestTimeout = timeout) {
  toolCalls++;
  const response = await rpc.request('mcpServer/tool/call', { threadId: thread, server: selected.name, tool: name, arguments: args }, requestTimeout);
  // Inspect provider signals internally. Persist only a category, never text.
  const texts = (response?.content ?? []).filter(c => c.type === 'text').map(c => c.text).join('\n');
  if (response?.isError && /rate.?limit|ratelimited|too many requests|retry.after|\b429\b/i.test(texts)) throw fault('linear_throttled');
  return { value: parseTool(response), bytes: Buffer.byteLength(JSON.stringify(response)) };
}
async function newSession() {
  if (stopping) throw fault(stopping);
  const begin = performance.now();
  const rpc = new RPC(binary, childArgs, repo);
  allRPCs.add(rpc);
  managedChildren.push(rpc.process);
  startedProcesses++;
  const threadIDs = [];
  const phase = {};
  try {
    let beginPhase = performance.now();
    await rpc.request('initialize', { clientInfo: { name: 'wtf_linear_concurrency_spike', version: '1' }, capabilities: { experimentalApi: true } }, startupTimeout);
    rpc.send({ method: 'initialized', params: {} });
    phase.initialize_ms = performance.now() - beginPhase;
    for (let index = 0; index < threads; index++) {
      beginPhase = performance.now();
      const started = await rpc.request('thread/start', { cwd: repo, ephemeral: true, approvalPolicy: 'never', sandbox: 'read-only', baseInstructions: '', developerInstructions: '', serviceName: 'wtf-linear-concurrency-spike' }, startupTimeout);
      if (!started.thread?.id || started.thread.ephemeral !== true) throw fault('ephemeral_connection_required');
      phase[`thread_${index}_start_ms`] = performance.now() - beginPhase;
      beginPhase = performance.now();
      const { value: workspace } = await tool(rpc, started.thread.id, 'get_workspace', {}, startupTimeout);
      const parsed = new URL(workspace.url);
      const slug = parsed.pathname.split('/').filter(Boolean)[0];
      if (parsed.protocol !== 'https:' || parsed.hostname !== 'linear.app' || !workspace.id || slug !== expected) throw fault('workspace_mismatch');
      phase[`thread_${index}_verification_ms`] = performance.now() - beginPhase;
      threadIDs.push(started.thread.id);
    }
    const session = { rpc, threadIDs, setup_ms: performance.now() - begin, phases: phase, account: expected };
    sessions.push({ setup_ms: session.setup_ms, phases: phase, account: expected });
    return session;
  } catch (error) {
    await rpc.close();
    allRPCs.delete(rpc);
    throw error;
  }
}
async function closeSession(session) {
  await session.rpc.close();
  allRPCs.delete(session.rpc);
}
async function lookup(session, thread, index, round, roundStarted) {
  if (stopping) return;
  const start = performance.now();
  active++;
  peakActive = Math.max(peakActive, active);
  let code = 'review_found', bytes = 0;
  try {
    const result = await tool(session.rpc, thread, 'get_diff', { urlOrId: urls[index] });
    bytes = result.bytes;
    const diff = result.value;
    if (diff.url !== urls[index] || !diff.appUrl) throw fault('review_mapping_mismatch');
    const review = new URL(diff.appUrl);
    if (review.protocol !== 'https:' || review.hostname !== 'linear.app' || review.port || review.username || review.password || !review.pathname.startsWith(`/${expected}/review/`) || /[\x00-\x20\x7f-\x9f]/.test(diff.appUrl)) throw fault('invalid_review_url');
  } catch (error) {
    code = reason(error);
    if (code !== 'review_not_found') stop(code);
  } finally {
    active--;
    outcomes.push({ index, round, code, latency_ms: performance.now() - start, completed_ms: performance.now() - roundStarted, cold_completed_ms: performance.now() - startedAt, response_bytes: bytes });
  }
}

const roundMetrics = [];
const bulkMetrics = [];
let failure = null;
try {
  if (mode === 'list') {
    const session = await newSession();
    const roundStarted = performance.now();
    const repositories = [...new Set(urls.map(url => new URL(url).pathname.split('/').slice(1, 3).join('/')))];
    const matches = new Map();
    const collect = value => {
      if (!value || typeof value !== 'object') return;
      if (typeof value.url === 'string' && urls.includes(value.url) && typeof value.appUrl === 'string') {
        const review = new URL(value.appUrl);
        if (review.protocol !== 'https:' || review.hostname !== 'linear.app' || review.port || review.username || review.password || !review.pathname.startsWith(`/${expected}/review/`) || /[\x00-\x20\x7f-\x9f]/.test(value.appUrl)) throw fault('invalid_review_url');
        matches.set(value.url, true);
      }
      for (const child of Object.values(value)) if (child && typeof child === 'object') collect(child);
    };
    await Promise.all(repositories.map(async repository => {
      const [owner, repoName] = repository.split('/');
      const start = performance.now();
      const result = await tool(session.rpc, session.threadIDs[0], 'list_diffs', { owner, repo: repoName, limit: 50, orderBy: 'updatedAt' });
      collect(result.value);
      bulkMetrics.push({ repository, latency_ms: performance.now() - start, bytes: result.bytes, response_keys: Object.keys(result.value), hasNextPage: result.value.hasNextPage ?? null });
    }));
    const fallback = [];
    for (let index = 0; index < jobs; index++) {
      if (matches.has(urls[index])) outcomes.push({ index, round: 0, code: 'review_found_bulk', latency_ms: performance.now() - roundStarted, completed_ms: performance.now() - roundStarted, cold_completed_ms: performance.now() - startedAt, response_bytes: 0 });
      else fallback.push(index);
    }
    let next = 0;
    await Promise.all(Array.from({ length: concurrency }, async () => {
      while (!stopping && next < fallback.length) { const index = fallback[next++]; await lookup(session, session.threadIDs[0], index, 0, roundStarted); }
    }));
    roundMetrics.push({ round: 0, lookup_ms: performance.now() - roundStarted, completed: outcomes.length, bulk_matches: matches.size, exact_fallbacks: fallback.length });
    await closeSession(session);
  } else if (mode === 'pool') {
    const pool = await Promise.all(Array.from({ length: processes }, () => newSession()));
    for (let round = 0; round < rounds && !stopping; round++) {
      const roundStarted = performance.now();
      let next = 0;
      await Promise.all(pool.flatMap(session => session.threadIDs.flatMap(thread => Array.from({ length: concurrency }, async () => {
        while (!stopping && next < jobs) { const index = next++; await lookup(session, thread, index, round, roundStarted); }
      }))));
      roundMetrics.push({ round, lookup_ms: performance.now() - roundStarted, completed: outcomes.filter(x => x.round === round).length });
      if (round + 1 < rounds) await new Promise(resolve => setTimeout(resolve, 2000));
    }
    await Promise.all(pool.map(closeSession));
  } else {
    const roundStarted = performance.now();
    let next = 0;
    await Promise.all(Array.from({ length: processes }, async () => {
      while (!stopping && next < jobs) {
        const index = next++;
        const session = await newSession();
        await lookup(session, session.threadIDs[0], index, 0, roundStarted);
        await closeSession(session);
      }
    }));
    roundMetrics.push({ round: 0, lookup_ms: performance.now() - roundStarted, completed: outcomes.length });
  }
} catch (error) {
  failure = reason(error);
  stop(failure);
} finally {
  clearInterval(sampler);
  clearTimeout(deadline);
  if (cancelTimer) clearTimeout(cancelTimer);
  await Promise.all([...allRPCs].map(async rpc => { await rpc.close(); allRPCs.delete(rpc); }));
  await Promise.all(managedChildren.filter(child => child.exitCode === null && child.signalCode === null).map(child => new Promise(resolve => {
    const timer = setTimeout(resolve, 1000);
    child.once('exit', () => { clearTimeout(timer); resolve(); });
  })));
}
const pct = (values, fraction) => {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted.length ? Math.round(sorted[Math.ceil(sorted.length * fraction) - 1]) : null;
};
const statuses = {};
for (const outcome of outcomes) statuses[outcome.code] = (statuses[outcome.code] ?? 0) + 1;
const report = {
  timestamp: new Date().toISOString(), layout: { mode, processes, threads, concurrency, jobs, rounds },
  deadlines: { lookup_ms: timeout, startup_ms: startupTimeout, injected_cancellation_ms: options.has('--cancel-after-ms') ? Number(options.get('--cancel-after-ms')) : null },
  account: { profile, server: selected.name, expected_workspace: expected },
  bridge_sha256: createHash('sha256').update(bridge).digest('hex'), unique_inputs: new Set(urls).size,
  wall_ms: Math.round(performance.now() - startedAt), discovery_ms: Math.round(discoveryMS),
  app_servers_started: startedProcesses, peak_lookup_concurrency: peakActive, tool_calls: toolCalls,
  statuses, stopping, failure, completed: outcomes.length,
  cleanup: { managed_children: managedChildren.length, alive_children: managedChildren.filter(child => child.exitCode === null && child.signalCode === null).length },
  successful_within_cold_8s: outcomes.filter(x => x.round === 0 && ['review_found', 'review_not_found', 'review_found_bulk'].includes(x.code) && x.cold_completed_ms <= 8000).length,
  latency_p50_ms: pct(outcomes.map(x => x.latency_ms), .5), latency_p95_ms: pct(outcomes.map(x => x.latency_ms), .95),
  setup_p50_ms: pct(sessions.map(x => x.setup_ms), .5), resources, rounds: roundMetrics, bulk: bulkMetrics, sessions, outcomes,
  limitations: ['One Node coordinator; fresh mode restarts app-server per PR but shares configuration discovery and Node runtime', 'Existing Codex-owned OAuth; no credentials extracted and no inference turns', 'System-wide swap deltas can include unrelated activity', 'No provider quota headers available through this RPC interface', 'Confirmed missing review is successful association coverage; other errors stop escalation', 'Repeated inputs and warm rounds are intentional stress workloads, not production deduplication'],
};
writeFileSync(options.get('--output'), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(Object.fromEntries(Object.entries(report).filter(([key]) => !['sessions', 'outcomes', 'limitations'].includes(key)))));
if (stopping || failure || outcomes.length !== jobs * rounds) process.exitCode = 1;
