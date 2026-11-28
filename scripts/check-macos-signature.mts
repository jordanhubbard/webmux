#!/usr/bin/env node
// Read-only release gate. Displays WebMux.app twice and never signs it.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import {
  assertCDHashUnchanged,
  assertDisplayArgsAreReadOnly,
  assertReleaseSignature,
  missingStableSignatureCredential,
  signatureDisplayArgs,
} from './macos-app.mts';

function display(target: string): string {
  const args = signatureDisplayArgs(target);
  assertDisplayArgsAreReadOnly(args);
  const result = spawnSync('/usr/bin/codesign', args, { encoding: 'utf8' });
  return `${result.stdout ?? ''}\n${result.stderr ?? ''}`;
}

function fail(message: string): never {
  console.error(message);
  if (!message.includes('Missing credential:')) console.error(missingStableSignatureCredential);
  process.exit(1);
}

function main(): void {
  if (process.platform !== 'darwin') {
    fail(`macOS signature check cannot succeed on ${process.platform}. Run it on macOS against WebMux.app. It only displays the signature and does not sign.`);
  }
  const target = path.resolve(process.argv[2] ?? path.join(os.homedir(), 'Applications', 'WebMux.app'));
  if (!fs.existsSync(target)) fail(`No WebMux.app at ${target}.`);
  try {
    const first = display(target);
    assertReleaseSignature(first);
    assertCDHashUnchanged(first, display(target));
  } catch (error) {
    fail(error instanceof Error ? error.message : String(error));
  }
  console.log(`Stable signature at ${target}. CDHash unchanged across two read-only checks.`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main();
