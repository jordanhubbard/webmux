#!/usr/bin/env node
// Exercise an extracted bundle with isolated state, including native runtime modules.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, type ChildProcess } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
import { once } from 'node:events';

async function main(): Promise<void> {
  const argument = process.argv[2];
  if (!argument) throw new Error('Usage: node scripts/smoke-package.mts <bundle-directory>');
  const root = path.resolve(argument);
  const argon2: typeof import('argon2') = require(path.join(root, 'node_modules/argon2'));
  const pty: typeof import('node-pty') = require(path.join(root, 'node_modules/node-pty'));
  assert(await argon2.verify(await argon2.hash('package-test'), 'package-test'));
  await new Promise<void>((resolve, reject) => {
    const windows = process.platform === 'win32';
    const terminal = pty.spawn(windows ? (process.env.ComSpec || 'cmd.exe') : '/bin/sh',
      windows ? ['/d', '/s', '/c', 'echo webmux-pty-ok'] : ['-c', 'printf webmux-pty-ok'],
      { env: process.env });
    let output = '';
    const timeout = setTimeout(() => { terminal.kill(); reject(new Error('PTY timed out')); }, 10000);
    terminal.onData(data => { output += data; });
    terminal.onExit(({ exitCode }) => {
      clearTimeout(timeout);
      try { assert.equal(exitCode, 0); assert.match(output, /webmux-pty-ok/); resolve(); }
      catch (error) { reject(error); }
    });
  });
  const socket = net.createServer();
  socket.listen(0, '127.0.0.1');
  await once(socket, 'listening');
  const address = socket.address();
  assert(address && typeof address === 'object');
  const port = address.port;
  await new Promise<void>((resolve, reject) => socket.close(error => error ? reject(error) : resolve()));
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-smoke-'));
  const config = path.join(home, 'config');
  fs.mkdirSync(config);
  const app = fs.readFileSync(path.join(root, 'config.defaults/app.yaml'), 'utf8')
    .replace('name: webmux', 'name: package-preservation-test')
    .replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1');
  fs.writeFileSync(path.join(config, 'app.yaml'), app);
  let child: ChildProcess | undefined;
  let logs = '';
  try {
    for (let attempt = 0; attempt < 2; attempt++) {
      const windows = process.platform === 'win32';
      const command = windows ? process.execPath : path.join(root, 'bin/webmux');
      const args = windows ? [path.join(root, 'bin/webmux.js')] : [];
      child = spawn(command, args, {
        cwd: home,
        env: { ...process.env, WEBMUX_HOME: home, WEBMUX_ROOT: '/nonexistent', HTTP_PORT: String(port) },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      const exited = once(child, 'exit');
      child.stdout?.on('data', (data: Buffer) => { logs += data; });
      child.stderr?.on('data', (data: Buffer) => { logs += data; });
      let healthy = false;
      for (let retry = 0; retry < 100; retry++) {
        if (child.exitCode !== null) throw new Error(`Server exited: ${logs}`);
        try {
          const response = await fetch(`http://127.0.0.1:${port}/api/health`);
          const health: unknown = await response.json();
          assert(health && typeof health === 'object' && 'status' in health && 'name' in health);
          assert.equal(health.status, 'ok');
          assert.equal(health.name, 'package-preservation-test');
          healthy = true;
          break;
        } catch { await new Promise(resolve => setTimeout(resolve, 100)); }
      }
      assert(healthy, `Server did not start: ${logs}`);
      const response = await fetch(`http://127.0.0.1:${port}/`);
      assert.equal(response.status, 200);
      assert.match(await response.text(), /<div id="root">/);
      assert.equal(fs.readFileSync(path.join(config, 'app.yaml'), 'utf8'), app);
      assert(fs.existsSync(path.join(config, 'auth.yaml')));
      child.kill('SIGTERM');
      const timer = setTimeout(() => child?.kill('SIGKILL'), 10000);
      const [code, signal] = await exited;
      clearTimeout(timer);
      if (windows) assert(code !== null || signal, logs);
      else assert.equal(code, 0, logs);
      child = undefined;
    }
    assert(!fs.existsSync(path.join(root, 'node_modules/typescript')), 'Bundle includes development dependencies');
    assert(!fs.existsSync(path.join(root, 'config')), 'Runtime wrote configuration inside the bundle');
    console.log('Bundle smoke test passed: HTTP, frontend, native modules, restart, preserved config.');
  } finally {
    if (child) child.kill('SIGKILL');
    fs.rmSync(home, { recursive: true, force: true });
  }
}
// ConPTY may retain an internal pipe handle after onExit fires. This is a bounded
// command-line smoke test, so exit explicitly after every assertion and cleanup.
main().then(
  () => process.exit(0),
  error => { console.error(error); process.exit(1); },
);
