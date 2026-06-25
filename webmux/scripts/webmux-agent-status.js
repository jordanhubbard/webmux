#!/usr/bin/env node

const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const AGENT_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/;
const VALID_STATUSES = new Set(['waiting', 'working', 'unknown', 'stale']);

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith('--')) continue;
    const key = arg.slice(2).replace(/-/g, '_');
    const value = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[++i] : 'true';
    args[key] = value;
  }
  return args;
}

function tmuxSocketArgs(socket) {
  if (!socket) return [];
  return path.isAbsolute(socket) ? ['-S', socket] : ['-L', socket];
}

function tmuxSocketPathFromEnv(value) {
  if (!value) return undefined;
  return value.split(',')[0] || undefined;
}

function tmuxSessionFromPane(pane, socket) {
  if (!pane) return undefined;
  const displayArgs = ['display-message', '-p', '-t', pane, '#S'];
  const attempts = [];
  const configuredSocketArgs = tmuxSocketArgs(socket);
  if (configuredSocketArgs.length) attempts.push([...configuredSocketArgs, ...displayArgs]);
  const tmuxEnvSocketArgs = tmuxSocketArgs(tmuxSocketPathFromEnv(process.env.TMUX));
  if (tmuxEnvSocketArgs.length) attempts.push([...tmuxEnvSocketArgs, ...displayArgs]);
  attempts.push(displayArgs);

  for (const args of attempts) {
    try {
      const output = execFileSync('tmux', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
      if (output) return output;
    } catch {
      // Try the next tmux addressing mode.
    }
  }
  return undefined;
}

function resolveSessionName(args) {
  return args.name ||
    process.env.WEBMUX_AGENT_SESSION ||
    tmuxSessionFromPane(process.env.TMUX_PANE, args.tmux_socket || process.env.WEBMUX_AGENT_TMUX_SOCKET);
}

function encodeSessionName(name) {
  return Buffer.from(name, 'utf8')
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '');
}

function statusFile(agentId, name) {
  const webmuxHome = process.env.WEBMUX_HOME || path.join(os.homedir(), '.config', 'webmux');
  return path.join(webmuxHome, 'data', 'agent-status', agentId, `${encodeSessionName(name)}.json`);
}

function readJson(file) {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch {
    return {};
  }
}

function writeJsonAtomic(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const tmp = `${file}.tmp-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  fs.writeFileSync(tmp, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
  fs.renameSync(tmp, file);
}

async function readStdin() {
  let input = '';
  for await (const chunk of process.stdin) {
    input += chunk.toString('utf8');
  }
  if (!input.trim()) return {};
  try {
    return JSON.parse(input);
  } catch {
    return {};
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const agentId = args.agent || args.kind || 'codex';
  const status = args.status;

  if (!AGENT_ID_RE.test(agentId) || !VALID_STATUSES.has(status)) return;

  const hookInput = await readStdin();
  const name = resolveSessionName(args);
  if (!name) {
    console.error('webmux-agent-status: could not resolve tmux session name');
    return;
  }

  const now = new Date().toISOString();
  const file = statusFile(agentId, name);
  const previous = readJson(file);
  const next = {
    ...previous,
    agent_id: agentId,
    name,
    status,
    source: 'hook',
    updated_at: now,
    hook_session_id: hookInput.session_id || previous.hook_session_id,
    hook_turn_id: hookInput.turn_id || previous.hook_turn_id,
  };

  if (status === 'waiting') {
    next.last_ready_at = now;
    next.last_output_at = now;
    delete next.last_output_source;
  } else if (status === 'working') {
    next.last_input_at = now;
  }

  writeJsonAtomic(file, next);
}

if (require.main === module) {
  main().catch(err => {
    console.error(`webmux-agent-status: ${err.message}`);
  });
}

module.exports = {
  tmuxSessionFromPane,
  tmuxSocketPathFromEnv,
};
