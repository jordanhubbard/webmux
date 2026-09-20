#!/usr/bin/env node

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const AGENT_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/;
const VALID_STATUSES = new Set(['waiting', 'working', 'unknown', 'stale']);

function parseArgs(argv: string[]): Record<string, string | undefined> {
  const args: Record<string, string | undefined> = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith('--')) continue;
    const key = arg.slice(2).replace(/-/g, '_');
    const value = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[++i] : 'true';
    args[key] = value;
  }
  return args;
}

function tmuxSocketArgs(socket: string | undefined): string[] {
  if (!socket) return [];
  return path.isAbsolute(socket) ? ['-S', socket] : ['-L', socket];
}

export function tmuxSocketPathFromEnv(value: string | undefined): string | undefined {
  if (!value) return undefined;
  return value.split(',')[0] || undefined;
}

export function tmuxSessionFromPane(pane: string | undefined, socket: string | undefined): string | undefined {
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

function resolveSessionName(args: Record<string, string | undefined>): string | undefined {
  return args.name ||
    process.env.WEBMUX_AGENT_SESSION ||
    tmuxSessionFromPane(process.env.TMUX_PANE, args.tmux_socket || process.env.WEBMUX_AGENT_TMUX_SOCKET);
}

function encodeSessionName(name: string): string {
  return Buffer.from(name, 'utf8')
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '');
}

function statusFile(agentId: string, name: string): string {
  const webmuxHome = process.env.WEBMUX_HOME || path.join(os.homedir(), '.config', 'webmux');
  return path.join(webmuxHome, 'data', 'agent-status', agentId, `${encodeSessionName(name)}.json`);
}

function parseObject(input: string): Record<string, unknown> {
  const value: unknown = JSON.parse(input);
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown> : {};
}

function readJson(file: string): Record<string, unknown> {
  try {
    return parseObject(fs.readFileSync(file, 'utf8'));
  } catch {
    return {};
  }
}

function writeJsonAtomic(file: string, value: Record<string, unknown>): void {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const tmp = `${file}.tmp-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  fs.writeFileSync(tmp, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
  fs.renameSync(tmp, file);
}

async function readStdin(): Promise<Record<string, unknown>> {
  let input = '';
  for await (const chunk of process.stdin) {
    const value: unknown = chunk;
    if (Buffer.isBuffer(value)) input += value.toString('utf8');
    else if (typeof value === 'string') input += value;
  }
  if (!input.trim()) return {};
  try {
    return parseObject(input);
  } catch {
    return {};
  }
}

async function main(): Promise<void> {
  const args = parseArgs(process.argv.slice(2));
  const agentId = args.agent || args.kind || 'codex';
  const status = args.status;

  if (!AGENT_ID_RE.test(agentId) || !status || !VALID_STATUSES.has(status)) return;

  const hookInput = await readStdin();
  const name = resolveSessionName(args);
  if (!name) {
    console.error('webmux-agent-status: could not resolve tmux session name');
    return;
  }

  const now = new Date().toISOString();
  const file = statusFile(agentId, name);
  const previous = readJson(file);
  const next: Record<string, unknown> = {
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
  main().catch((err: unknown) => {
    console.error(`webmux-agent-status: ${err instanceof Error ? err.message : String(err)}`);
  });
}
