#!/usr/bin/env node
// Checked release/Homebrew CI helpers shared by Unix and Windows packaging.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';

function record(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value), 'Expected a JSON object');
  return value as Record<string, unknown>;
}
function readJSON(file: string): unknown { return JSON.parse(fs.readFileSync(file, 'utf8')); }
function environment(name: string): string {
  const value = process.env[name]; assert(value, `Missing ${name}`); return value;
}
function first(value: unknown): unknown {
  assert(Array.isArray(value) && value.length > 0, 'Expected a nonempty JSON array'); return value[0];
}
const repo = path.resolve(import.meta.dirname, '..');
switch (process.argv[2]) {
  case 'release-version': {
    const version = record(readJSON(path.join(repo, 'webmux/backend/package.json'))).version;
    assert.equal(typeof version, 'string');
    assert.equal(environment('RELEASE_TAG'), `v${String(version)}`, 'Release tag must match the backend version');
    break;
  }
  case 'tap': {
    const value = record(readJSON(path.join(environment('RUNNER_TEMP'), 'webmux-tap-info.json')));
    assert.equal(record(first(value.formulae)).full_name, 'jordanhubbard/webmux/webmux');
    break;
  }
  case 'formula': {
    const source = path.join(environment('RUNNER_TEMP'), 'webmux-source.tar.gz');
    const file = environment('WEBMUX_TEST_FORMULA');
    const sha = createHash('sha256').update(fs.readFileSync(source)).digest('hex');
    const formula = fs.readFileSync(file, 'utf8');
    assert.match(formula, /^  url .*$/m); assert.match(formula, /^  sha256 .*$/m);
    // JSON string quoting also escapes paths safely in this Ruby string literal.
    const url = JSON.stringify(`file://${source}`).replaceAll('#', '\\#');
    fs.writeFileSync(file, formula.replace(/^  url .*$/m, () => `  url ${url}\n  version "0.0.0"`)
      .replace(/^  sha256 .*$/m, () => `  sha256 "${sha}"`));
    break;
  }
  case 'service': {
    const value = record(first(readJSON(path.join(environment('RUNNER_TEMP'), 'webmux-service.json'))));
    assert(typeof value.command === 'string' && value.command.endsWith('/opt/webmux/bin/webmux'));
    assert(typeof value.working_dir === 'string' && value.working_dir.length > 0);
    assert(typeof value.log_path === 'string' && value.log_path.endsWith('/var/log/webmux.log'));
    break;
  }
  default: throw new Error('Usage: node scripts/packaging-checks.mts <release-version|tap|formula|service>');
}
