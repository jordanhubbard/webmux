import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(process.argv[2] || path.join(import.meta.dirname, '..'));
const lock = JSON.parse(fs.readFileSync(path.join(root, 'package-lock.json'), 'utf8')) as {
  packages: Record<string, { version?: string; optional?: boolean; link?: boolean }>;
};
let checked = 0;
for (const [location, entry] of Object.entries(lock.packages)) {
  if (!location.includes('node_modules/') || entry.link) continue;
  const manifest = path.join(root, location, 'package.json');
  if (!fs.existsSync(manifest) && entry.optional) continue;
  assert(fs.existsSync(manifest), `Missing package: ${location}; run npm ci`);
  const actual = JSON.parse(fs.readFileSync(manifest, 'utf8')) as { name: string; version: string };
  const expectedName = location.slice(location.lastIndexOf('node_modules/') + 13);
  assert.equal(actual.name, expectedName, `Package identity mismatch: ${location}`);
  assert.equal(actual.version, entry.version, `Lock version mismatch: ${location}`);
  checked++;
}
assert(!fs.existsSync(path.join(root, 'frontend/package-lock.json')), 'Use the workspace lockfile only');
console.log(`Verified ${checked} installed package identities against the workspace lockfile.`);
