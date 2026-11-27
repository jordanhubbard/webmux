#!/usr/bin/env node
// Configure fresh or existing homes without shell substitutions in YAML values.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(new URL('../webmux/package.json', import.meta.url));
const yaml = require('js-yaml') as { load(text: string): unknown; dump(value: unknown): string };
const home = process.env.WEBMUX_HOME;
assert(home && path.isAbsolute(home), 'WEBMUX_HOME must be absolute');
const defaults = path.resolve(import.meta.dirname, '../webmux/config.defaults');
const updates: Record<string, Record<string, string | number | boolean>> = { app: {}, auth: {} };
for (const [variable, section, field] of [
  ['LISTEN_HOST', 'app', 'listen_host'], ['HTTP_PORT', 'app', 'http_port'],
  ['HTTPS_PORT', 'app', 'https_port'], ['SECURE_MODE', 'app', 'secure_mode'],
  ['AUTH_MODE', 'auth', 'mode'],
]) {
  const value = process.env[variable];
  if (!value) continue;
  if (variable.endsWith('_PORT')) {
    assert(/^\d+$/.test(value) && Number(value) <= 65535, `${variable} must be an integer from 0 to 65535`);
    updates[section][field] = Number(value);
  } else if (variable === 'SECURE_MODE') {
    assert(value === 'true' || value === 'false', 'SECURE_MODE must be true or false');
    updates[section][field] = value === 'true';
  } else {
    if (variable === 'AUTH_MODE') assert(['none', 'local'].includes(value), 'AUTH_MODE must be none or local');
    updates[section][field] = value;
  }
}
const directory = path.join(home, 'config');
// Parse both files before writing either, so malformed YAML leaves them intact.
const files = Object.entries(updates).map(([section, values]) => {
  const filename = path.join(directory, `${section}.yaml`);
  const exists = fs.existsSync(filename);
  const document = yaml.load(fs.readFileSync(exists ? filename : path.join(defaults, `${section}.yaml`), 'utf8'));
  assert(document && typeof document === 'object' && !Array.isArray(document), `${section}.yaml must contain a mapping`);
  const config = document as Record<string, unknown>;
  const fields = config[section];
  assert(fields && typeof fields === 'object' && !Array.isArray(fields), `${section}.yaml must contain a ${section} mapping`);
  Object.assign(fields, values);
  return { filename, section, fields, contents: yaml.dump(config), write: !exists || Object.keys(values).length > 0 };
});
fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
for (const file of files) {
  if (file.write) fs.writeFileSync(file.filename, file.contents, { mode: 0o600 });
  console.log(`${file.section}.yaml:`);
  for (const field of Object.keys(file.section === 'app'
    ? { listen_host: 0, http_port: 0, https_port: 0, secure_mode: 0 } : { mode: 0 })) {
    console.log(`  ${field}: ${JSON.stringify((file.fields as Record<string, unknown>)[field])}`);
  }
}
