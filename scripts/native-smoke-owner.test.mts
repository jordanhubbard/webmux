import { test } from 'node:test';
import assert from 'node:assert/strict';
import { admitWindowsOwnerReceipt } from './native-smoke-owner.mts';

const stopped = { version: 1, phase: 'stopped', rootPID: 123, requested: true, totalProcesses: 3, activeProcesses: 0, observedDescendants: 2, liveDescendants: 0 };
test('Windows shutdown admission requires zero active processes in the owned job', () => {
  admitWindowsOwnerReceipt(stopped);
  for (const value of [null, {}, { ...stopped, phase: 'running' }, { ...stopped, phase: 'timeout' },
    { ...stopped, activeProcesses: 1 }, { ...stopped, liveDescendants: 1 }, { ...stopped, observedDescendants: undefined }, { ...stopped, requested: false },
    { ...stopped, rootPID: 0 }, { ...stopped, totalProcesses: 0 }, { ...stopped, version: 2 }]) {
    assert.throws(() => admitWindowsOwnerReceipt(value));
  }
});
