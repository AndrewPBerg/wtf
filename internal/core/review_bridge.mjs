#!/usr/bin/env node
// Embedded review adapter: direct MCP calls only; never starts a model turn.
import { spawn, spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { homedir } from 'node:os';
import { realpathSync } from 'node:fs';

const input = process.argv.slice(2);
const options = new Map();
for (let i = 0; i < input.length; i += 2) {
  const key = input.at(i), value = input.at(i + 1);
  if (!['--repo', '--pr', '--server', '--expect-workspace', '--timeout-ms', '--profile', '--doctor'].includes(key) || !value) {
    if (key === '--help') {
      process.stdout.write('Usage: node review_bridge.mjs --repo PATH (--pr GITHUB_PR_URL | --doctor true) [--profile alita|cascading|plain] [--server NAME] [--expect-workspace SLUG] [--timeout-ms 20000]\n');
      process.exit(0);
    }
    process.stdout.write(JSON.stringify({ ok: false, error: 'invalid_arguments' }) + '\n');
    process.exit(2);
  }
  options.set(key, value);
}

function fault(code) { return Object.assign(new Error(code), { code }); }
function parseTool(result) {
  const texts = (result?.content ?? []).filter(c => c.type === 'text').map(c => c.text);
  if (result?.isError) throw fault(texts.some(t => /diff not found/i.test(t)) ? 'review_not_found' : 'linear_tool_error');
  if (result?.structuredContent && typeof result.structuredContent === 'object') return result.structuredContent;
  for (const text of texts) {
    try { return JSON.parse(text); } catch {}
    if (/^Error:\s*Diff not found\s*$/i.test(text)) throw fault('review_not_found');
  }
  throw fault('invalid_linear_response');
}

class RPC {
  constructor(binary, args, cwd) {
    this.pending = new Map();
    this.nextId = 1;
    // Inherit the Go parent's process group so cancellation cleans up the bridge
    // and Codex together. Only the selected HTTP MCP connection is started.
    this.process = spawn(binary, args, { cwd, stdio: ['pipe', 'pipe', 'pipe'] });
    this.process.stdin.on('error', () => this.rejectAll('codex_write_failed'));
    // Never print server diagnostics: they can contain account or credential data.
    this.process.stderr.resume();
    this.lines = createInterface({ input: this.process.stdout });
    this.lines.on('line', line => {
      let message;
      try { message = JSON.parse(line); } catch { return; }
      const waiter = this.pending.get(message.id);
      if (waiter && ('result' in message || 'error' in message)) {
        this.pending.delete(message.id);
        clearTimeout(waiter.timer);
        if (message.error) waiter.reject(fault(`rpc_error:${waiter.method}:${message.error.code}`));
        else waiter.resolve(message.result);
      } else if (message.method && message.id !== undefined) {
        // No approval, authentication, or write interaction is performed by this resolver.
        this.send({ id: message.id, error: { code: -32601, message: 'Read-only review resolver does not handle interactive requests' } });
      }
    });
    this.process.on('error', () => this.rejectAll('codex_start_failed'));
    this.process.on('exit', code => this.rejectAll(`codex_exited:${code}`));
  }
  send(value) { if (!this.process.stdin.destroyed) this.process.stdin.write(JSON.stringify(value) + '\n'); }
  request(method, params, timeout) {
    return new Promise((resolve, reject) => {
      const id = this.nextId++;
      const timer = setTimeout(() => { this.pending.delete(id); reject(fault(`timeout:${method}`)); }, timeout);
      this.pending.set(id, { resolve, reject, timer, method });
      this.send({ method, params, id });
    });
  }
  rejectAll(code) {
    for (const waiter of this.pending.values()) { clearTimeout(waiter.timer); waiter.reject(fault(code)); }
    this.pending.clear();
  }
  async close() {
    this.lines.close();
    this.process.stdin.end();
    const terminate = signal => {
      if (this.process.exitCode !== null || !this.process.pid) return;
      try { this.process.kill(signal); } catch {}
    };
    terminate('SIGTERM');
    await new Promise(resolve => {
      if (this.process.exitCode !== null) return resolve();
      const timer = setTimeout(() => { terminate('SIGKILL'); resolve(); }, 1000);
      this.process.once('exit', () => { clearTimeout(timer); resolve(); });
    });
  }
}

let rpc;
let output;
try {
  const repo = realpathSync(resolve(options.get('--repo') ?? process.cwd()));
  const doctor = options.get('--doctor') === 'true';
  let githubUrl;
  if (!doctor) {
    const parsed = new URL(options.get('--pr') ?? '');
    if (parsed.protocol !== 'https:' || parsed.hostname !== 'github.com' || parsed.port || parsed.username || parsed.password || parsed.search || parsed.hash || !/^\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pull\/[1-9][0-9]*\/?$/.test(parsed.pathname)) throw fault('invalid_pr_url');
    githubUrl = `https://github.com${parsed.pathname.replace(/\/$/, '')}`;
  }
  const under = root => repo === root || repo.startsWith(root + '/');
  const inferredProfile = under(resolve(homedir(), 'Desktop/Work')) ? 'alita' : under(resolve(homedir(), 'Desktop/cl')) ? 'cascading' : null;
  const configuredProfile = options.get('--profile') ?? inferredProfile;
  const profile = configuredProfile === 'plain' ? null : configuredProfile;
  const profileArgs = profile ? ['--profile', profile] : [];
  output = { ok: true, mode: doctor ? 'doctor' : 'resolve', provider: doctor ? 'linear' : 'github', url: githubUrl, githubUrl, repo, profile, server: null, workspace: null, reason: 'linear_unavailable' };
  const timeout = Number(options.get('--timeout-ms') ?? '20000');
  if (!Number.isInteger(timeout) || timeout < 1000 || timeout > 60000) throw fault('invalid_timeout');
  const locate = spawnSync('mise', ['which', 'codex'], { encoding: 'utf8', timeout: 5000 });
  const binary = locate.status === 0 ? locate.stdout.trim() : 'codex';
  const discover = spawnSync(binary, [...profileArgs, 'mcp', 'list', '--json'], { cwd: repo, encoding: 'utf8', timeout: 5000, maxBuffer: 2 * 1024 * 1024 });
  if (discover.status !== 0) throw fault('codex_config_unavailable');
  const servers = JSON.parse(discover.stdout);
  const candidates = servers.filter(s => s.enabled && s.transport?.type === 'streamable_http' && s.transport.url === 'https://mcp.linear.app/mcp');
  const selected = options.has('--server') ? candidates.find(s => s.name === options.get('--server')) : candidates.length === 1 ? candidates.at(0) : null;
  if (!selected) throw fault(candidates.length > 1 ? 'ambiguous_linear_connection' : 'linear_connection_missing');
  output.server = selected.name;
  const expected = options.get('--expect-workspace') ?? ({ linear: 'cascadinglabs', 'linear-alita': 'alitahealth' })[selected.name];
  if (!expected) throw fault('workspace_expectation_required');
  output.expectedWorkspace = expected;
  // This Codex app-server rejects named profile switches. Discovery still resolves
  // that profile; carry only its selected public MCP connection into app-server.
  // The OAuth credentials remain in Codex's existing store, keyed by connection.
  if (selected.auth_status !== 'o_auth' || selected.transport.bearer_token_env_var || Object.keys(selected.transport.http_headers ?? {}).length || Object.keys(selected.transport.env_http_headers ?? {}).length) throw fault('unsupported_connection_auth');
  const args = [];
  for (const server of servers) {
    if (!/^[A-Za-z0-9_-]+$/.test(server.name)) throw fault('unsupported_server_identifier');
    if (server.name === selected.name) continue;
    const disabled = server.transport?.type === 'stdio'
      ? '{command="/usr/bin/false",args=[],enabled=false}'
      : '{url="https://mcp.linear.app/mcp",enabled=false}';
    args.push('-c', `mcp_servers.${server.name}=${disabled}`);
  }
  args.push('-c', `mcp_servers.${selected.name}={url="https://mcp.linear.app/mcp",enabled=true,enabled_tools=["get_workspace","get_diff"]}`);
  args.push('app-server', '--stdio');
  rpc = new RPC(binary, args, repo);
  await rpc.request('initialize', { clientInfo: { name: 'wtf_linear_reviews', version: '1' }, capabilities: { experimentalApi: true } }, timeout);
  rpc.send({ method: 'initialized', params: {} });
  const started = await rpc.request('thread/start', { cwd: repo, ephemeral: true, approvalPolicy: 'never', sandbox: 'read-only', baseInstructions: '', developerInstructions: '', serviceName: 'wtf-linear-reviews' }, timeout);
  const threadId = started.thread?.id;
  if (!threadId || started.thread?.ephemeral !== true) throw fault('ephemeral_connection_required');
  const tool = async (name, arguments_) => parseTool(await rpc.request('mcpServer/tool/call', { threadId, server: selected.name, tool: name, arguments: arguments_ }, timeout));
  const workspace = await tool('get_workspace', {});
  const slug = new URL(workspace.url).pathname.split('/').filter(Boolean).at(0);
  output.workspace = { id: workspace.id, name: workspace.name, slug };
  if (!workspace.id || slug !== expected) throw fault('workspace_mismatch');
  if (doctor) {
    const inventory = await rpc.request('mcpServerStatus/list', { threadId, serverName: selected.name, detail: 'toolsAndAuthOnly', limit: 1 }, timeout);
    const connection = inventory.data?.find(entry => entry.name === selected.name);
    const tools = Object.keys(connection?.tools ?? {});
    if (!['get_workspace', 'get_diff'].every(name => tools.includes(name) || tools.some(key => key.endsWith('__' + name)))) throw fault('review_tools_unavailable');
    output.tools = ['get_workspace', 'get_diff'];
    output.authStatus = connection.authStatus;
    output.connectionStatus = connection.runtimeStatus;
    output.status = 'ready';
    output.reason = 'connection_ready';
  } else {
    const diff = await tool('get_diff', { urlOrId: githubUrl });
    if (diff.url !== githubUrl || !diff.appUrl) throw fault('review_mapping_mismatch');
    const reviewUrl = new URL(diff.appUrl);
    if (reviewUrl.protocol !== 'https:' || reviewUrl.hostname !== 'linear.app' || reviewUrl.port || !reviewUrl.pathname.startsWith(`/${slug}/review/`) || /[\x00-\x20\x7f-\x9f]/.test(diff.appUrl)) throw fault('invalid_review_url');
    output.provider = 'linear';
    output.url = diff.appUrl;
    output.reason = 'review_found';
  }
} catch (error) {
  const reason = error?.code ?? 'resolver_failed';
  if (output) {
    output.reason = reason;
    if (output.mode === 'doctor') { output.ok = false; output.status = 'unavailable'; process.exitCode = 1; }
  }
  else { output = { ok: false, error: reason }; process.exitCode = 2; }
} finally {
  if (rpc) await rpc.close();
}
output.checkedAt = new Date().toISOString();
process.stdout.write(JSON.stringify(output) + '\n');
