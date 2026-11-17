import assert from 'node:assert/strict';
import { test } from 'node:test';
import { nativeAbsenceProof } from '../tests/e2e/rig-native-absence.mts';

test('native detach admission rejects stale, ambiguous, foreign-pane and uncertain observations', () => {
  const row = { canonicalSessionName: 'seat', sessionStatus: 'detached', lifecycleState: 'detached',
    identityVerdict: { sessionName: 'seat', verdict: 'pane_missing', reason: 'session_missing', observedAt: '2026-11-17T08:22:42.143Z', evidence: { registeredPane: '%3' } } };
  const after = Date.parse('2026-11-17T08:22:40Z');
  const admit = (value: unknown) => nativeAbsenceProof(value, 'seat', '%3', after);
  assert.deepEqual(admit([row]), { session: 'seat', pane: '%3', observedAt: row.identityVerdict.observedAt });
  for (const value of [null, {}, [], [row, row], [{ ...row, canonicalSessionName: 'foreign' }],
    [{ ...row, sessionStatus: 'running' }], [{ ...row, lifecycleState: 'attention_required' }], [{ ...row, identityVerdict: null }],
    ...[{ observedAt: '2026-11-17T08:22:39Z' }, { observedAt: 'invalid' }, { sessionName: 'foreign' },
      { verdict: 'mismatch' }, { reason: 'process_identity_ambiguous' }, { evidence: { registeredPane: '%2' } }, { evidence: null }]
      .map(change => [{ ...row, identityVerdict: { ...row.identityVerdict, ...change } }]),
  ]) assert.equal(admit(value), null, JSON.stringify(value));
});
