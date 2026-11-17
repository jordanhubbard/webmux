import assert from 'node:assert/strict';
import fs from 'node:fs';
import { once } from 'node:events';
import type { ChildProcess } from 'node:child_process';

export function admitWindowsOwnerReceipt(value: unknown): void {
  assert(value && typeof value === 'object' && !Array.isArray(value), 'Missing ownership receipt');
  const receipt = value as Record<string, unknown>;
  assert.equal(receipt.version, 1, 'Unknown ownership receipt version');
  assert.equal(receipt.phase, 'stopped', 'Qualification job has not stopped');
  assert.equal(receipt.requested, true, 'Qualification root exited before its stop request');
  assert(Number.isSafeInteger(receipt.rootPID) && Number(receipt.rootPID) > 0, 'Missing owned root identity');
  assert(Number.isSafeInteger(receipt.totalProcesses) && Number(receipt.totalProcesses) >= 1, 'Missing job accounting');
  assert.equal(receipt.activeProcesses, 0, 'Qualification job has surviving processes');
  assert(Number.isSafeInteger(receipt.observedDescendants) && Number(receipt.observedDescendants) >= 0, 'Missing descendant observation');
  assert.equal(receipt.liveDescendants, 0, 'Observed descendants have not exited');
}

export async function stopWindowsOwner(child: ChildProcess, receiptPath: string): Promise<void> {
  const closed = child.exitCode === null && child.signalCode === null ? once(child, 'close') : Promise.resolve([child.exitCode, child.signalCode]);
  let timeout: NodeJS.Timeout | undefined;
  try {
    if (child.exitCode === null && child.signalCode === null) {
      assert(child.stdin?.writable, 'Qualification owner has no control pipe');
      // EOF alone is a failed control, so the complete explicit command must
      // precede it. The root application receives NUL, never this control pipe.
      child.stdin.once('error', () => {}); // The exit/receipt checks still reject lost control.
      child.stdin.end('stop\n');
    }
    const [code, signal] = await Promise.race([
      closed,
      new Promise<never>((_resolve, reject) => {
        timeout = setTimeout(() => {
          child.kill('SIGKILL'); // Closing the owner handle kills its own Job Object.
          reject(new Error('Qualification owner exceeded its shutdown deadline'));
        }, 15_000);
      }),
    ]);
    assert.equal(code, 0, `Qualification owner exit: ${String(signal)}`);
    const receipt: unknown = JSON.parse(fs.readFileSync(receiptPath, 'utf8'));
    admitWindowsOwnerReceipt(receipt);
    console.log(`Windows qualification ownership: ${JSON.stringify(receipt)}`);
  } finally { clearTimeout(timeout); }
}
