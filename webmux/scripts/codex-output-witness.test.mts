import { test } from 'node:test';
import assert from 'node:assert/strict';
import { OutputWitness } from './codex-output-witness.mts';
const watches = Array.from({ length: 4 }, (_, i) => ({ thread: `${i}`.repeat(36), nonce: `${i}`.repeat(32) }));
const event = (delta: string, overrides = {}) => ({ method: 'item/commandExecution/outputDelta', params: { threadId: watches[0].thread, turnId: 'turn', itemId: 'item', delta, ...overrides } });
test('only owned command stdout counts, retaining exact thread, turn and item identity', () => {
  const witness = new OutputWitness(watches);
  assert.equal(witness.accept({ ...event('WM_CODING_'+watches[0].nonce+'_9\n'), method: 'model/prose' }), false);
  assert.equal(witness.accept(event('WM_CODING_'+watches[0].nonce+'_9\n', { threadId: 'other' })), false);
  assert.equal(witness.accept(event('echo WM_CODING_'+watches[0].nonce+'_9\n')), false);
  assert.equal(witness.accept(event('WM_CODING_'+watches[1].nonce+'_9\n')), false);
  assert.equal(witness.accept(event('WM_CODING_'+watches[0].nonce+'_')), false);
  assert.equal(witness.accept(event('2\r\n')), true);
  assert.equal(witness.accept(event('WM_CODING_'+watches[0].nonce+'_1\n')), false);
  assert.deepEqual(witness.rows(), [{ threadId: watches[0].thread, turnId: 'turn', itemId: 'item', counter: 2 }]);
  witness.accept(event('WM_CODING_'+watches[0].nonce+'_99\n', { turnId: 'other', itemId: 'other' }));
  assert.equal(witness.rows()[0].counter, 2);
});
test('command count is bounded and invalid watches are refused', () => {
  assert.throws(() => new OutputWitness(watches.slice(1)));
  const witness = new OutputWitness(watches);
  for (let i = 0; i < 64; i++) witness.accept(event('x'.repeat(70000), { itemId: `${i}` }));
  assert.throws(() => witness.accept(event('', { itemId: 'overflow' })));
});
