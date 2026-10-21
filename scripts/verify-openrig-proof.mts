import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

function object(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value), 'Expected an evidence object');
  return value as Record<string, unknown>;
}
function text(value: unknown): string { assert.equal(typeof value, 'string'); return value as string; }
function positivePid(value: unknown) { assert(typeof value === 'number' && Number.isInteger(value) && value > 0, 'Missing live process PID'); }

export function verifyOpenRigProof(input: unknown) {
  const proof = object(input);
  assert.equal(proof.schema_version, 1);
  const run = text(proof.run_id);
  assert.match(run, /^webmux-live-[0-9a-f]{16}$/);
  const controller = object(proof.controller);
  const daemon = object(proof.daemon);
  const health = object(daemon.health);
  assert.equal(daemon.mock, false, 'A daemon fixture does not qualify live coordination');
  assert.equal(daemon.implementation, '@openrig/cli');
  assert.equal(health.status, 'ok');
  assert.equal(daemon.version, health.semver);
  positivePid(health.pid);
  const sandbox = object(proof.codex_sandbox);
  assert.equal(sandbox.mode, 'workspace-write', 'Proof must retain Codex filesystem isolation');
  assert.equal(sandbox.network_access, true);
  const alphaName = 'proof-alpha@webmux-proof';
  const betaName = 'proof-beta@webmux-proof';
  assert.equal(controller.seed_target, alphaName, 'Only ALPHA receives the controller challenge');
  assert(Array.isArray(proof.agents) && proof.agents.length === 2);
  const pids = new Set<number>();
  for (const [name, runtime, command] of [[alphaName, 'claude-code', 'claude'], [betaName, 'codex', 'codex']]) {
    const agent = object(proof.agents.find(value => object(value).session === name));
    assert.equal(agent.runtime, runtime);
    const process = object(agent.process);
    positivePid(process.pid);
    pids.add(process.pid as number);
    assert.equal(process.command, command, 'Expected a native provider CLI');
    assert(text(process.version).length > 0);
  }
  assert.equal(pids.size, 2, 'Expected distinct CLI processes');
  const alpha = object(proof.alpha);
  const beta = object(proof.beta);
  for (const result of [alpha, beta]) {
    assert.equal(result.verified, true);
    assert.equal(result.run_id, run);
    assert.equal(result.nonce_a, controller.nonce_a);
    assert.match(text(result.nonce_a), /^[0-9a-f]{48}$/);
    assert.match(text(result.nonce_b), /^[0-9a-f]{48}$/);
    assert.notEqual(result.nonce_a, result.nonce_b);
  }
  assert.equal(alpha.runtime, 'claude-code');
  assert.equal(beta.runtime, 'codex');
  assert.equal(alpha.peer, betaName);
  assert.equal(beta.peer, alphaName);
  assert.equal(alpha.nonce_b, beta.nonce_b);
  const digest = createHash('sha256').update(`webmux-openrig-v1:${run}:${alpha.nonce_a}:${alpha.nonce_b}`).digest('hex');
  assert.equal(alpha.digest, digest, 'ALPHA digest must independently recompute');
  assert.equal(beta.digest, digest, 'BETA digest must independently recompute');
  assert(Array.isArray(proof.messages) && proof.messages.length === 3, 'Need challenge, response and ACK');
  const messages = proof.messages.map(object);
  const ids = new Set<string>();
  let previous = -Infinity;
  for (const [index, message] of messages.entries()) {
    const sender = index === 1 ? betaName : alphaName;
    const destination = index === 1 ? alphaName : betaName;
    assert.equal(message.sender_session, sender);
    assert.equal(message.destination_session, destination);
    assert.equal(message.identity_provenance, 'transport:v1', 'Missing transport-derived seat identity');
    ids.add(text(message.outbox_id));
    const timestamp = Date.parse(text(message.ts_dispatched));
    assert(timestamp > previous, 'Messages must occur in protocol order');
    previous = timestamp;
    const body = text(message.body);
    assert(body.startsWith(`From: ${sender}\nTo: ${destination}\n`), 'Envelope identity mismatch');
    assert(body.includes(run));
    const payload = body.split('\n---\n')[1];
    assert(payload, 'Missing captured message payload');
    if (index === 0) { assert(payload.includes(text(alpha.nonce_a))); continue; }
    const decoded = object(JSON.parse(payload));
    assert.equal(decoded.type, index === 1 ? 'response' : 'ack');
    assert.equal(decoded.run_id, run);
    assert.equal(decoded.digest, digest);
    if (index === 1) {
      assert.equal(decoded.nonce_a, alpha.nonce_a);
      assert.equal(decoded.nonce_b, alpha.nonce_b);
    }
  }
  assert.equal(ids.size, 3, 'Messages need distinct daemon outbox records');
  return { run, digest, messages: messages.length };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const file = process.argv[2] ?? path.resolve(import.meta.dirname, '../docs/validation/openrig-live-2026-10-21/proof.json');
  const result = verifyOpenRigProof(JSON.parse(fs.readFileSync(file, 'utf8')));
  console.log(`PASS: Claude Code ↔ Codex, live OpenRig challenge → response → ACK (${result.run}); SHA-256 ${result.digest}`);
}
