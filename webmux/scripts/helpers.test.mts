import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';

test('compiled agent hook preserves session metadata and status transitions', () => {
  const home = mkdtempSync(path.join(os.tmpdir(), 'webmux-hook-test-'));
  try {
    const script = path.join(import.meta.dirname, 'dist', 'webmux-agent-status.js');
    const name = 'session/with unicode 🔐';
    const file = path.join(home, 'data', 'agent-status', 'codex', `${Buffer.from(name).toString('base64url')}.json`);
    const run = (status: string, input: string) => {
      const result = spawnSync(process.execPath, [script, '--agent', 'codex', '--name', name, '--status', status], {
        env: { ...process.env, WEBMUX_HOME: home }, input, encoding: 'utf8', timeout: 10_000,
      });
      assert.equal(result.status, 0, result.stderr);
      const value: unknown = JSON.parse(readFileSync(file, 'utf8'));
      assert.ok(value && typeof value === 'object' && !Array.isArray(value));
      return value as Record<string, unknown>;
    };
    const waiting = run('waiting', '{"session_id":"fixture-session","turn_id":"fixture-turn"}');
    assert.equal(waiting.name, name);
    assert.equal(waiting.status, 'waiting');
    assert.equal(waiting.last_ready_at, waiting.updated_at);
    assert.equal(waiting.last_output_at, waiting.updated_at);
    const working = run('working', '{}');
    assert.equal(working.status, 'working');
    assert.equal(working.hook_session_id, 'fixture-session');
    assert.equal(working.hook_turn_id, 'fixture-turn');
    assert.equal(working.last_ready_at, waiting.last_ready_at);
    assert.equal(working.last_input_at, working.updated_at);
    const invalidInput = run('waiting', 'null');
    assert.equal(invalidInput.hook_session_id, 'fixture-session');
    assert.equal(readdirSync(path.dirname(file)).length, 1, 'temporary status files leaked');
  } finally { rmSync(home, { recursive: true, force: true }); }
});
