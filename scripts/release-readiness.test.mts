import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { releaseWorkflows, requireWorkflowSuccess } from './release-readiness.mts';

const head = 'a'.repeat(40);
const run = (values: Record<string, unknown> = {}) => ({
  databaseId: 2, workflowName: 'CI', headSha: head, headBranch: 'main', event: 'push',
  createdAt: '2026-01-02T00:00:00Z', status: 'completed', conclusion: 'success', ...values,
});

test('release gates name the currently tracked workflows', () => {
  for (const workflow of releaseWorkflows) {
    const text = fs.readFileSync(path.resolve(import.meta.dirname, '../.github/workflows', workflow.file), 'utf8');
    assert.equal(text.split(/\r?\n/)[0], `name: ${workflow.name}`);
  }
});

test('latest exact-main run wins regardless of API ordering or an earlier success', () => {
  const earlier = run({ databaseId: 1, createdAt: '2026-01-01T00:00:00Z' });
  for (const conclusion of ['failure', 'cancelled', 'timed_out', 'skipped', 'neutral', 'action_required']) {
    const latest = run({ conclusion });
    for (const values of [[earlier, latest], [latest, earlier]]) {
      assert.throws(() => requireWorkflowSuccess(values, head, 'CI'), /successful completion is required/);
    }
  }
  requireWorkflowSuccess([run({ conclusion: 'cancelled', databaseId: 1, createdAt: earlier.createdAt }), run()], head, 'CI');
  requireWorkflowSuccess([run({ event: 'workflow_dispatch' })], head, 'CI');
});

test('missing, pending, foreign or malformed workflow observations cannot admit publication', () => {
  const cases: unknown[] = [null, [], {}, [null], [run(), run()], [run({ headSha: 'b'.repeat(40) })],
    [run({ headBranch: 'release/v1.2.4' })], [run({ event: 'pull_request' })], [run({ workflowName: 'Other' })],
    [run({ status: 'in_progress', conclusion: null })], [run({ status: 'queued', conclusion: 'success' })],
    [run({ createdAt: 'not a date' })], [run({ databaseId: 0 })], [run({ conclusion: undefined })],
  ];
  for (const value of cases) assert.throws(() => requireWorkflowSuccess(value, head, 'CI'));
});
