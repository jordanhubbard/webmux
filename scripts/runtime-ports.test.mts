import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { runtimePorts } from './runtime-ports.mts';

function fixture(t: { after(fn: () => void): void }) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-port priority-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'state & config'), defaults = path.join(root, 'defaults.yaml');
  fs.writeFileSync(defaults, 'app:\n  http_port: 8080\n  https_port: 8443\n');
  const configured = path.join(home, 'config/app.yaml');
  const write = (text: string) => { fs.mkdirSync(path.dirname(configured), { recursive: true }); fs.writeFileSync(configured, text); };
  return { root, home, defaults, configured, write };
}

test('runtime ports prefer explicit values, then the saved file, then defaults without writes', t => {
  const f = fixture(t);
  assert.deepEqual(runtimePorts(f.home, f.defaults), { http: 8080, https: 8443 });
  assert(!fs.existsSync(f.home), 'resolving ports created user configuration');
  const source = 'app:\n  http_port: 19271\n  https_port: 0\n  custom: retained\n'; f.write(source);
  assert.deepEqual(runtimePorts(f.home, f.defaults), { http: 19271, https: 0 });
  assert.deepEqual(runtimePorts(f.home, f.defaults, { HTTP_PORT: '20100' }), { http: 20100, https: 0 });
  assert.deepEqual(runtimePorts(f.home, f.defaults, { HTTP_PORT: '', HTTPS_PORT: '22100' }), { http: 19271, https: 22100 });
  assert.deepEqual(runtimePorts(f.home, f.defaults, { HTTP_PORT: '000', HTTPS_PORT: '+0' }), { http: 0, https: 0 });
  assert.equal(fs.readFileSync(f.configured, 'utf8'), source);
  f.write('app: {}\n');
  assert.deepEqual(runtimePorts(f.home, f.defaults), { http: 0, https: 0 }, 'existing partial files must match the native zero values');
});

test('invalid selected ports and malformed files refuse instead of starting with defaults', t => {
  const f = fixture(t); f.write('app:\n  http_port: 19271\n  https_port: 0\n');
  for (const bad of ['-1', '65536', '1.5', 'private-port-value', '$(shell echo unexpected)']) {
    assert.throws(() => runtimePorts(f.home, f.defaults, { HTTP_PORT: bad }), /Invalid runtime port configuration/);
  }
  for (const content of ['', 'app: [', 'app: []\n', 'app:\n  http_port: private-port-value\n', 'app:\n  http_port: -1\n']) {
    f.write(content); assert.throws(() => runtimePorts(f.home, f.defaults));
    assert.equal(fs.readFileSync(f.configured, 'utf8'), content);
  }
  f.write('app:\n  http_port: -1\n  https_port: 65536\n');
  assert.deepEqual(runtimePorts(f.home, f.defaults, { HTTP_PORT: '19271', HTTPS_PORT: '0' }), { http: 19271, https: 0 });
});

test('operator file links are read and preserved; dangling links cannot silently use defaults', { skip: process.platform === 'win32' }, t => {
  const f = fixture(t);fs.mkdirSync(path.dirname(f.configured), { recursive: true });
  const target = path.join(f.root, 'operator.yaml'); fs.writeFileSync(target, 'app:\n  http_port: 19271\n  https_port: 0\n');
  fs.symlinkSync(target, f.configured); assert.deepEqual(runtimePorts(f.home, f.defaults), { http: 19271, https: 0 });
  assert(fs.lstatSync(f.configured).isSymbolicLink()); fs.unlinkSync(target);
  assert.throws(() => runtimePorts(f.home, f.defaults)); assert(fs.lstatSync(f.configured).isSymbolicLink());
});

test('CLI emits only numeric ports and redacts private malformed configuration', t => {
  const f = fixture(t);f.write('app:\n  http_port: 19271\n  https_port: 0\n');
  const script = path.join(import.meta.dirname, 'runtime-ports.mts');
  const run = () => spawnSync(process.execPath, [script], { encoding: 'utf8', timeout: 10000,
    env: { ...process.env, WEBMUX_HOME: f.home, WEBMUX_ROOT: path.resolve(import.meta.dirname, '../webmux'), HTTP_PORT: '', HTTPS_PORT: '' } });
  const good=run();assert.ifError(good.error);assert.equal(good.status,0,good.stderr);assert.equal(good.stdout,'19271 0\n');assert.equal(good.stderr,'');
  const content='app:\n  http_port: [PRIVATE_CONFIGURATION_VALUE\n';f.write(content);
  const refused=run();assert.ifError(refused.error);assert.equal(refused.status,1);assert.equal(refused.stdout,'');
  assert.match(refused.stderr,/Cannot resolve listener ports/);assert(!refused.stderr.includes('PRIVATE_CONFIGURATION_VALUE'));
  assert.equal(fs.readFileSync(f.configured,'utf8'),content);
});
