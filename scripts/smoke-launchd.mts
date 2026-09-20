#!/usr/bin/env node
// Exercise the shipped native launchd template under a private, temporary label.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { renderService } from './render-service.mts';

assert.equal(process.platform, 'darwin', 'launchd smoke requires macOS');
const root = path.resolve(import.meta.dirname, '../webmux');
await fs.access(path.join(root, 'bin/webmux'));
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'webmux-launchd-'));
const home = path.join(temporary, 'state & config');
const label = `com.webmux.smoke.${process.pid}.${randomBytes(6).toString('hex')}`;
const uid = process.getuid!();
function launch(args: string[], required = true): string {
  const result = spawnSync('launchctl', args, { encoding: 'utf8', timeout: 15000 });
  if (result.error) throw result.error;
  if (required) assert.equal(result.status, 0, `launchctl ${args.join(' ')}: ${result.stderr}`);
  return result.status === 0 ? result.stdout : '';
}
const domain = launch(['print', `gui/${uid}`], false) ? `gui/${uid}` : `user/${uid}`;
const target = `${domain}/${label}`;
const plist = path.join(temporary, 'fixture.plist');
let registered = false;
async function waitFor(check: () => Promise<boolean>, description: string): Promise<void> {
  const until = Date.now() + 15000;
  while (!await check()) {
    assert(Date.now() < until, `Timed out: ${description}`);
    await new Promise(resolve => setTimeout(resolve, 50));
  }
}
try {
  assert.equal(launch(['print', target], false), '', 'Fixture service label already exists');
  await fs.cp(path.join(root, 'config.defaults'), path.join(home, 'config'), { recursive: true });
  await fs.mkdir(path.join(home, 'logs'), { recursive: true });
  const listener = net.createServer(); listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
  const address = listener.address(); assert(address && typeof address === 'object');
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  const appFile = path.join(home, 'config/app.yaml');
  const app = (await fs.readFile(appFile, 'utf8')).replace('name: webmux', `name: ${label}`)
    .replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1')
    .replace('http_port: 8080', `http_port: ${address.port}`).replace('https_port: 8443', 'https_port: 0');
  await fs.writeFile(appFile, app);
  await fs.writeFile(path.join(home, 'config/auth.yaml'), 'auth:\n  mode: none\n  users: []\n');
  const xml = (value: string) => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
  let template = renderService(await fs.readFile(path.join(root, 'service/com.webmux.server.plist.native.template'), 'utf8'), 'darwin', {
    root, home, node: process.execPath, searchPath: process.env.PATH ?? '/usr/bin:/bin',
  }).replace('<string>com.webmux.server</string>', `<string>${label}</string>`);
  // Prevent launchd's environment from selecting operator ports or slave mode.
  const environment = { HTTP_PORT: String(address.port), HTTPS_PORT: '0', JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '' };
  template = template.replace('<key>EnvironmentVariables</key>\n    <dict>', '<key>EnvironmentVariables</key>\n    <dict>\n' +
    Object.entries(environment).map(([key, value]) => `<key>${key}</key><string>${xml(value)}</string>`).join('\n'));
  await fs.writeFile(plist, template);
  const lint = spawnSync('plutil', ['-lint', plist], { encoding: 'utf8' });
  assert.equal(lint.status, 0, lint.stdout + lint.stderr);
  // Fail before registration if a template edit stopped any isolation rewrite.
  for (const [key, expected] of Object.entries({ Label: label, 'EnvironmentVariables.WEBMUX_HOME': home,
    'EnvironmentVariables.HTTP_PORT': String(address.port), 'EnvironmentVariables.WEBMUX_SLAVE_HOST': '' })) {
    const extracted = spawnSync('plutil', ['-extract', key, 'raw', '-o', '-', plist], { encoding: 'utf8' });
    assert.equal(extracted.status, 0, extracted.stderr); assert.equal(extracted.stdout.trim(), expected);
  }
  const base = `http://127.0.0.1:${address.port}`;
  async function healthy(): Promise<boolean> {
    try {
      const response = await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) });
      const value: unknown = await response.json();
      return response.ok && typeof value === 'object' && value !== null && 'name' in value && value.name === label;
    } catch { return false; }
  }
  registered = true;
  launch(['bootstrap', domain, plist]);
  let auth = '';
  let previousPID = '';
  for (let round = 0; round < 2; round++) {
    if (round) launch(['kickstart', target]);
    await waitFor(healthy, 'native launchd startup');
    const service = launch(['print', target]);
    assert.match(service, /state = running/);
    const pid = service.match(/\bpid = ([1-9][0-9]*)/)?.[1];
    assert(pid); assert.notEqual(pid, previousPID, 'Restart reused the prior process'); previousPID = pid;
    const response = await fetch(base, { signal: AbortSignal.timeout(5000) });
    assert(response.ok); assert.match(await response.text(), /<html/i);
    assert.equal(await fs.readFile(appFile, 'utf8'), app, 'Service rewrote application configuration');
    const current = await fs.readFile(path.join(home, 'config/auth.yaml'), 'utf8');
    assert.match(current, /jwt_secret:/);
    if (round) assert.equal(current, auth, 'Signing secret changed across service restart');
    auth = current;
    launch(['kill', 'SIGTERM', target]);
    await waitFor(async () => !await healthy(), 'service listener shutdown');
    await waitFor(async () => {
      const stopped = launch(['print', target]);
      return /last exit code = 0/.test(stopped) && !/\bpid = [1-9]/.test(stopped) && !/state = running/.test(stopped);
    }, 'graceful native exit');
  }
  console.log(`Native launchd template passed startup, UI, graceful stop and restart in ${domain}`);
} finally {
  if (registered && launch(['print', target], false)) {
    launch(['bootout', target]);
    assert.equal(launch(['print', target], false), '', 'Fixture service remains registered');
  }
  await fs.rm(temporary, { recursive: true, force: true });
}
