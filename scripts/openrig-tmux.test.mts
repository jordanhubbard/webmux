import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

test('coordination refuses unsafe tmux before touching the daemon or existing seats', { skip: process.platform === 'win32' }, () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-tmux-gate-'));
  const runner = path.resolve(import.meta.dirname, '../webmux/server/internal/openrigcoord/runner.mts');
  try {
    const bin = path.join(root, 'bin'); fs.mkdirSync(bin);
    const calls = path.join(root, 'rig-called');
    fs.writeFileSync(path.join(bin, 'tmux'), `#!${process.execPath}\nconsole.log(process.env.WM_TMUX_VERSION);\n`, { mode: 0o700 });
    fs.writeFileSync(path.join(bin, 'rig'), `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(calls)}, 'called'); console.log('Daemon running on port 1');\n`, { mode: 0o700 });
    for (const version of ['tmux 2.9a', 'tmux 3.2a', 'tmux 3.3a', 'unrecognized']) {
      for (const action of ['ensure', 'status', 'create']) {
        const result = spawnSync(process.execPath, [runner, action, path.join(root, 'state')], {
          env: { ...process.env, PATH: bin + path.delimiter + process.env.PATH, WM_TMUX_VERSION: version }, encoding: 'utf8', timeout: 5000,
        });
        assert.equal(result.status, 1, version);
        assert.match(result.stderr, /requires tmux 3\.4 or newer/);
        assert.equal(fs.existsSync(calls), false, 'must not call Rig on an unsafe server');
      }
    }
    for (const version of ['tmux 3.4', 'tmux 3.6a', 'tmux next-3.7', 'tmux 4.0']) {
      const result = spawnSync(process.execPath, [runner, 'version-only', path.join(root, 'state')], {
        env: { ...process.env, PATH: bin + path.delimiter + process.env.PATH, WM_TMUX_VERSION: version }, encoding: 'utf8', timeout: 5000,
      });
      assert.match(result.stderr, /Unknown coordinator action/, version);
      assert.equal(fs.existsSync(calls), false);
    }
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
