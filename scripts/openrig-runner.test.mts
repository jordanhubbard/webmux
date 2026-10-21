// Unit fixtures for orchestration and verification. These do not qualify live
// agents; the real provider proof is recorded separately under docs/validation.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const runner = path.resolve(import.meta.dirname, '../webmux/server/internal/openrigproof/runner.mts');
const team = 'webmux-proof-1234567890abcdef12345678';
function fixture() {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-proof-unit-'));
  const bin = path.join(directory, 'bin'); fs.mkdirSync(bin);
  const stub = `#!${process.execPath}
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const args=process.argv.slice(2),cwd=process.cwd();
fs.appendFileSync(path.join(cwd,'calls'),JSON.stringify(args)+'\\n');
if(args[0]==='send'){
 const task=args[2],run=task.match(/Run id ([\\w-]+);/)[1],nonce=task.match(/nonce_a ([a-f0-9]+)\\./)[1];
 const nonce_b=crypto.randomBytes(24).toString('hex');
 const digest=crypto.createHash('sha256').update('webmux-openrig-v1:'+run+':'+nonce+':'+nonce_b).digest('hex');
 for(const seat of ['alpha','beta']){
  const value={run_id:process.env.WM_UNIT_MODE==='stale'?'old-run':run,nonce_a:nonce,nonce_b,digest:process.env.WM_UNIT_MODE==='tamper'?'0'.repeat(64):digest,verified:true,runtime:seat==='alpha'?'claude-code':'codex',peer:'proof-'+(seat==='alpha'?'beta':'alpha')+'@${team}'};
  fs.writeFileSync(path.join(cwd,seat,seat+'-proof.json'),JSON.stringify(value));
 }
}
console.log('unit fixture');
`;
  fs.writeFileSync(path.join(bin, 'rig'), stub, { mode: 0o700 });
  fs.writeFileSync(path.join(bin, 'tmux'), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
  return { directory, invoke: (action: string, mode = '') => spawnSync(process.execPath, [runner, action, directory, team], {
    env: { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH ?? ''}`, WM_UNIT_MODE: mode }, encoding: 'utf8', timeout: 15000,
  }), status: () => JSON.parse(fs.readFileSync(path.join(directory, 'status.json'), 'utf8')) as { phase: string; run_id: string; nonce_a: string; digest: string },
  calls: () => fs.readFileSync(path.join(directory, 'calls'), 'utf8').trim().split('\n').map(line => JSON.parse(line) as string[]),
  cleanup: () => fs.rmSync(directory, { recursive: true, force: true }) };
}

test('owner runner makes fresh challenges, keeps terminals alive, and explicitly stops only its team', { skip: process.platform === 'win32' }, () => {
  const f = fixture();
  try {
    const first = f.invoke('run'); assert.equal(first.status, 0, first.stdout + first.stderr);
    const a = f.status(); assert.equal(a.phase, 'passed');
    const second = f.invoke('run'); assert.equal(second.status, 0, second.stdout + second.stderr);
    const b = f.status(); assert.equal(b.phase, 'passed'); assert.notEqual(a.run_id, b.run_id); assert.notEqual(a.nonce_a, b.nonce_a);
    assert.equal(f.calls().filter(args => args[0] === 'send').length, 2);
    assert(f.calls().filter(args => args[0] === 'send').every(args => args[1] === `proof-alpha@${team}`));
    assert(!f.calls().some(args => ['down', 'daemon', 'setup'].includes(args[0])));
    assert.equal(f.invoke('stop').status, 0);
    assert.equal(f.status().phase, 'stopped');
    assert.deepEqual(f.calls().filter(args => args[0] === 'down'), [['down', team]]);
  } finally { f.cleanup(); }
});
for (const mode of ['tamper', 'stale']) test(`owner runner rejects ${mode} proof instead of showing PASS`, { skip: process.platform === 'win32' }, () => {
  const f = fixture();
  try { const result = f.invoke('run', mode); assert.notEqual(result.status, 0); assert.equal(f.status().phase, 'failed'); }
  finally { f.cleanup(); }
});
