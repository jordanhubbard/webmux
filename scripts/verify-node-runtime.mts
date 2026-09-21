#!/usr/bin/env node
// Shared checked-TypeScript validation for legacy Node runtime artifacts.
import assert from 'node:assert/strict';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
const require = createRequire(import.meta.url);

export async function verifyNodeRuntime(root: string): Promise<void> {
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
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const argument = process.argv[2];
  if (!argument) throw new Error('Usage: verify-node-runtime.mts <runtime-directory>');
  // Windows ConPTY can retain a handle after its exit event.
  verifyNodeRuntime(path.resolve(argument)).then(
    () => process.exit(0), error => { console.error(error); process.exit(1); },
  );
}
