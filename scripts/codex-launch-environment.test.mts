import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { captureCodexLaunchEnvironment, restoreCodexLaunchEnvironment, validateCodexLaunchEnvironment } from '../webmux/server/internal/openrigcoord/codex-launch-environment.mts';

const owner = { HOME: '/owner', PATH: '/owner/bin:/usr/bin', TMPDIR: '/owner/tmp', LANG: 'en_US.UTF-8',
  TOOL_API_KEY: 'PRIVATE_FIXTURE_VALUE_DO_NOT_SAVE', TOOL_OPTION: 'fixture-option',
  CODEX_THREAD_ID: 'parent-thread', CODEX_SESSION_ID: 'parent-session', TMUX_PANE: '%old', OPENRIG_NODE_ID: 'old-node' };
test('private environment stores standard values and only hashes other owner values', () => {
  const snapshot = captureCodexLaunchEnvironment(owner, '/native/home');
  assert.deepEqual(snapshot.values, { CODEX_HOME: '/native/home', HOME: owner.HOME, LANG: owner.LANG, PATH: owner.PATH, TMPDIR: owner.TMPDIR });
  assert.equal(JSON.stringify(snapshot).includes(owner.TOOL_API_KEY), false);
  assert.equal(JSON.stringify(snapshot).includes(owner.TOOL_OPTION), false);
  assert.equal(JSON.stringify(snapshot).includes('parent-thread'), false);
  assert.equal(snapshot.inheritedHashes.TOOL_API_KEY, createHash('sha256').update(owner.TOOL_API_KEY).digest('hex'));
});
test('restore uses exact owner values, fresh native bindings and no manager-only credentials', () => {
  const snapshot = captureCodexLaunchEnvironment(owner, '/native/home');
  const restored = restoreCodexLaunchEnvironment(snapshot, { ...owner, HOME: '/manager', PATH: '/manager/bin', TMPDIR: '/manager/tmp',
    OPENAI_API_KEY: 'MANAGER_ONLY_MUST_NOT_INHERIT', OPENRIG_NODE_ID: 'new-node', OPENRIG_OCCUPANT_GENERATION: 'new-generation', TMUX_PANE: '%new', TERM: 'xterm-256color' }, '/native/home', '/project');
  assert.equal(restored.HOME, owner.HOME); assert.equal(restored.PATH, owner.PATH); assert.equal(restored.TMPDIR, owner.TMPDIR);
  assert.equal(restored.TOOL_API_KEY, owner.TOOL_API_KEY); assert.equal(restored.OPENAI_API_KEY, undefined);
  assert.equal(restored.OPENRIG_NODE_ID, 'new-node'); assert.equal(restored.TMUX_PANE, '%new'); assert.equal(restored.TERM, 'xterm-256color');
  assert.equal(restored.PWD, '/project'); assert.equal(restored.CODEX_THREAD_ID, undefined); assert.equal(restored.CODEX_SESSION_ID, undefined);
});
test('missing or changed inherited values refuse without exposing their values or names', () => {
  const snapshot = captureCodexLaunchEnvironment(owner, '/native/home');
  for (const TOOL_API_KEY of [undefined, 'DIFFERENT_PRIVATE_VALUE']) {
    assert.throws(() => restoreCodexLaunchEnvironment(snapshot, { ...owner, TOOL_API_KEY }, '/native/home', '/project'), { message: 'codex_launch_environment_unavailable' });
  }
});
test('changed state, unknown plaintext fields and forged binding overrides are refused', () => {
  const snapshot = captureCodexLaunchEnvironment(owner, '/native/home');
  assert.throws(() => validateCodexLaunchEnvironment({ ...snapshot, values: { ...snapshot.values, PATH: '/changed' } }, '/native/home'));
  for (const name of ['TOOL_API_KEY', 'OPENRIG_NODE_ID']) {
    const { fingerprint: _old, ...content } = snapshot;
    const changed = { ...content, values: { ...content.values, [name]: 'forged-value' } };
    const fingerprint = createHash('sha256').update(JSON.stringify(changed)).digest('hex');
    assert.throws(() => validateCodexLaunchEnvironment({ ...changed, fingerprint }, '/native/home'));
  }
  assert.throws(() => validateCodexLaunchEnvironment(snapshot, '/another-home'));
});
test('malformed or oversized environments cannot become saved launch state', () => {
  for (const env of [{ PATH: '/bin' }, { ...owner, HOME: 'relative' }, { ...owner, INVALID: 'nul\0value' },
    { ...owner, ...Object.fromEntries(Array.from({ length: 513 }, (_, i) => ['KEY_' + i, 'value'])) }]) {
    assert.throws(() => captureCodexLaunchEnvironment(env, '/native/home'), { message: 'codex_launch_environment_unavailable' });
  }
});
