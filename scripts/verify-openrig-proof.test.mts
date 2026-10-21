import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { verifyOpenRigProof } from './verify-openrig-proof.mts';

const captured = JSON.parse(fs.readFileSync(path.resolve(import.meta.dirname, '../docs/validation/openrig-live-2026-10-21/proof.json'), 'utf8'));
test('live coordination proof checks the real captured exchange and rejects incomplete or inconsistent evidence', () => {
  assert.equal(verifyOpenRigProof(captured).messages, 3);
  const mutations = [
    (p: typeof captured) => { p.daemon.mock = true; },
    (p: typeof captured) => { p.agents[1].runtime = 'stub'; },
    (p: typeof captured) => { p.agents[1].process.pid = p.agents[0].process.pid; },
    (p: typeof captured) => { p.codex_sandbox.mode = 'danger-full-access'; },
    (p: typeof captured) => { p.beta.digest = '0'.repeat(64); },
    (p: typeof captured) => { p.messages.splice(1, 1); },
    (p: typeof captured) => { p.messages[1].identity_provenance = 'claimed:v1'; },
    (p: typeof captured) => { p.messages[1].sender_session = p.messages[0].sender_session; },
    (p: typeof captured) => { p.messages[2].body = p.messages[2].body.replace('"ack"', '"response"'); },
    (p: typeof captured) => { p.controller.seed_target = 'proof-beta@webmux-proof'; },
  ];
  for (const mutate of mutations) {
    const evidence = structuredClone(captured);
    mutate(evidence);
    assert.throws(() => verifyOpenRigProof(evidence));
  }
});
