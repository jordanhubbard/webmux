import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { renderService } from './render-service.mts';

const templates = path.resolve(import.meta.dirname, '../webmux/service');
for (const backend of ['node', 'go']) {
  test(`${backend} service preserves special path characters`, async () => {
    const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'webmux-service-render-'));
    const root = `${temporary}/app & 'quotes' "double" | %n $HOME \\ literal`;
    const home = `${temporary}/config & <data> %u $USER`;
    const node = `${temporary}/node executable`;
    const values = { root, home, node, searchPath: `${root}/bin:/usr/bin` };
    const suffix = backend === 'go' ? 'native.template' : 'template';
    try {
      const xml = renderService(await fs.readFile(path.join(templates, `com.webmux.server.plist.${suffix}`), 'utf8'), 'darwin', values);
      assert(xml.includes('&amp;')); assert(xml.includes('&lt;data&gt;')); assert(xml.includes('&quot;double&quot;'));
      if (process.platform === 'darwin') {
        const file = path.join(temporary, 'fixture.plist'); await fs.writeFile(file, xml);
        const parsed = spawnSync('plutil', ['-convert', 'json', '-o', '-', file], { encoding: 'utf8' });
        assert.equal(parsed.status, 0, parsed.stderr);
        const actual = JSON.parse(parsed.stdout);
        assert.deepEqual(actual.ProgramArguments, backend === 'go' ? [`${root}/bin/webmux`] : [node, `${root}/backend/dist/index.js`]);
        assert.equal(actual.WorkingDirectory, root);
        assert.deepEqual(actual.EnvironmentVariables, { WEBMUX_ROOT: root, WEBMUX_HOME: home, PATH: values.searchPath });
        assert.equal(actual.StandardOutPath, `${home}/logs/webmux.log`);
      }
      const unit = renderService(await fs.readFile(path.join(templates, `webmux.service.${suffix}`), 'utf8'), 'linux', values);
      assert(unit.includes('%%n')); assert(unit.includes('%%u'));
      assert(unit.includes('ExecStart=:"')); // Disable $ expansion, including in Node's script argument.
      assert(unit.includes('\\"double\\"'));
      assert(unit.includes(`WorkingDirectory=${root.replaceAll('%', '%%')}\n`));
      if (process.platform === 'linux') {
        await fs.mkdir(path.join(root, 'bin'), { recursive: true });
        await fs.mkdir(path.join(home, 'logs'), { recursive: true });
        await fs.symlink(process.execPath, node);
        await fs.symlink(process.execPath, path.join(root, 'bin/webmux'));
        const file = path.join(temporary, 'fixture.service'); await fs.writeFile(file, unit);
        const verified = spawnSync('systemd-analyze', ['verify', file], { encoding: 'utf8', timeout: 15000 });
        assert.ifError(verified.error); assert.equal(verified.status, 0, verified.stderr);
        assert.doesNotMatch(verified.stderr, /invalid|ignoring|failed|not executable/i);
      }
    } finally { await fs.rm(temporary, { recursive: true, force: true }); }
  });
}
test('service renderer rejects line and XML control characters', () => {
  for (const platform of ['darwin', 'linux'] as const) {
    for (const home of ['/tmp/a\nb', '/tmp/a\rb', '/tmp/a\0b']) {
      assert.throws(() => renderService('__WEBMUX_HOME__', platform, { root: '/tmp/root', home, node: '/usr/bin/node', searchPath: '/usr/bin' }), /control characters/);
    }
  }
});

test('installer CLI replaces definitions atomically and preserves them on invalid input', { skip: process.platform !== 'darwin' && process.platform !== 'linux' }, async () => {
  const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'webmux-service-cli-'));
  const output = path.join(temporary, 'definition');
  const home = path.join(temporary, 'state & config');
  try {
    await fs.writeFile(output, 'previous definition');
    const script = path.join(import.meta.dirname, 'render-service.mts');
    const result = spawnSync(process.execPath, [script, 'go'], { encoding: 'utf8',
      env: { ...process.env, WEBMUX_ROOT: path.resolve(templates, '..'), WEBMUX_HOME: home, WEBMUX_SERVICE_OUTPUT: output } });
    assert.equal(result.status, 0, result.stderr);
    const saved = await fs.readFile(output, 'utf8');
    assert(saved.includes('/bin/webmux')); assert(!saved.includes('backend/dist/index.js'));
    await fs.access(path.join(home, 'logs'));
    assert.equal((await fs.stat(output)).mode & 0o777, 0o600);
    const rejected = spawnSync(process.execPath, [script, 'go'], { encoding: 'utf8',
      env: { ...process.env, WEBMUX_ROOT: path.resolve(templates, '..'), WEBMUX_HOME: `${home}\ninvalid`, WEBMUX_SERVICE_OUTPUT: output } });
    assert.notEqual(rejected.status, 0);
    assert.equal(await fs.readFile(output, 'utf8'), saved);
    assert.deepEqual((await fs.readdir(temporary)).sort(), ['definition', 'state & config']);
  } finally { await fs.rm(temporary, { recursive: true, force: true }); }
});
