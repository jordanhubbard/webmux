#!/usr/bin/env node
// Keep release metadata edits out of shell interpolation and checked by tsc.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

function object(value: unknown, label: string): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value), `${label}: expected an object`);
  return value as Record<string, unknown>;
}
function read(file: string): Record<string, unknown> {
  return object(JSON.parse(fs.readFileSync(file, 'utf8')), file);
}
function version(value: Record<string, unknown>, label: string): string {
  assert(typeof value.version === 'string' && value.version.length > 0, `${label}: missing version`);
  return value.version;
}
function workspaceMetadata(root: string) {
  const rootFile = path.join(root, 'webmux/package.json');
  const frontendFile = path.join(root, 'webmux/frontend/package.json');
  const lockFile = path.join(root, 'webmux/package-lock.json');
  const rootPackage = read(rootFile), frontend = read(frontendFile), lock = read(lockFile);
  const packages = object(lock.packages, `${lockFile}: packages`);
  const lockedRoot = object(packages[''], `${lockFile}: root`);
  const lockedFrontend = object(packages.frontend, `${lockFile}: frontend`);
  return { rootFile, frontendFile, lockFile, rootPackage, frontend, lock, lockedRoot, lockedFrontend };
}

const [action, argument, rootArgument] = process.argv.slice(2);
switch (action) {
  case 'read': {
    assert(argument && !rootArgument, 'Usage: release-metadata.mts read <package.json>');
    console.log(version(read(argument), argument));
    break;
  }
  case 'validate': {
    assert(!rootArgument, 'Usage: release-metadata.mts validate [repository]');
    const value = workspaceMetadata(path.resolve(argument || '.'));
    const expected = version(value.rootPackage, 'package.json');
    for (const [label, doc] of [
      ['lock root version', value.lock], ['frontend/package.json', value.frontend], ['lock root', value.lockedRoot], ['lock frontend', value.lockedFrontend],
    ] as const) assert.equal(version(doc, label), expected, `Version mismatch: ${label} != package.json`);
    console.log(expected);
    break;
  }
  case 'bump': {
    assert(argument && /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(argument), 'Expected a major.minor.patch release version');
    const value = workspaceMetadata(path.resolve(rootArgument || '.'));
    // Load and validate every record before any write. Only workspace versions
    // change; dependency versions remain untouched.
    for (const doc of [value.rootPackage, value.frontend, value.lockedRoot, value.lockedFrontend]) {
      version(doc, 'workspace'); doc.version = argument;
    }
    value.lock.version = argument;
    for (const [file, doc] of [[value.rootFile, value.rootPackage], [value.frontendFile, value.frontend], [value.lockFile, value.lock]] as const) {
      fs.writeFileSync(file, JSON.stringify(doc, null, 2) + '\n');
    }
    break;
  }
  default: throw new Error('Usage: release-metadata.mts <read|validate|bump> [arguments]');
}
