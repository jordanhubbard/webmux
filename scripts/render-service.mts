#!/usr/bin/env node
// Serialize platform service values as data, without sed or shell expansion.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { randomBytes } from 'node:crypto';

export interface ServicePaths { root: string; home: string; searchPath: string }
export function renderService(template: string, platform: 'darwin' | 'linux', paths: ServicePaths): string {
  const values: Record<string, string> = { WEBMUX_DIR: paths.root, WEBMUX_HOME: paths.home, PATH: paths.searchPath };
  for (const value of Object.values(values)) assert(!/[\u0000-\u001f\u007f]/.test(value), 'Service paths must not contain control characters');
  const xml = (value: string) => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
  return template.split(/\r?\n/).map(line => line.replace(/__(WEBMUX_DIR|WEBMUX_HOME|PATH)__/g, (_, key: string) => {
    const value = values[key];
    if (platform === 'darwin') return xml(value);
    // All these directives expand systemd % specifiers. Only Environment and
    // ExecStart unquote words; WorkingDirectory/log paths take a whole value.
    const escaped = value.replaceAll('%', '%%');
    return /^(Environment|ExecStart)=/.test(line) ? escaped.replaceAll('\\', '\\\\').replaceAll('"', '\\"') : escaped;
  })).join('\n');
}

async function main(): Promise<void> {
  assert(process.platform === 'darwin' || process.platform === 'linux', 'Use the Windows service installer on Windows');
  const root = process.env.WEBMUX_ROOT ?? path.resolve(import.meta.dirname, '../webmux');
  assert(path.isAbsolute(root), 'WEBMUX_ROOT must be absolute');
  const home = process.env.WEBMUX_HOME, output = process.env.WEBMUX_SERVICE_OUTPUT;
  assert(home && path.isAbsolute(home), 'WEBMUX_HOME must be absolute');
  assert(output && path.isAbsolute(output), 'WEBMUX_SERVICE_OUTPUT must be absolute');
  const name = process.platform === 'darwin' ? 'com.webmux.server.plist' : 'webmux.service';
  const template = await fs.readFile(path.join(root, 'service', `${name}.native.template`), 'utf8');
  const rendered = renderService(template, process.platform, { root, home, searchPath: process.env.PATH ?? '/usr/bin:/bin' });
  await fs.mkdir(path.join(home, 'logs'), { recursive: true });
  await fs.mkdir(path.dirname(output), { recursive: true });
  const temporary = `${output}.${randomBytes(8).toString('hex')}.tmp`;
  try {
    await fs.writeFile(temporary, rendered, { flag: 'wx', mode: 0o600 });
    await fs.rename(temporary, output);
  } finally { await fs.rm(temporary, { force: true }); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) await main();
