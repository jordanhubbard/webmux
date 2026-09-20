import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { verifyNativeArtifacts } from './verify-native-artifacts.mts';

test('native upload gate rejects incomplete, stale, empty and corrupted release artifacts', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-native-artifacts-'));
  const archive = 'webmux-1.2.3-windows-arm64-native.zip';
  const installer = 'webmux-1.2.3-windows-arm64-native.msi';
  const verify = () => verifyNativeArtifacts(directory, '1.2.3', 'win32', 'arm64');
  const write = (name: string, bytes: string) => {
    fs.writeFileSync(path.join(directory, name), bytes);
    fs.writeFileSync(path.join(directory, `${name}.sha256`), `${createHash('sha256').update(bytes).digest('hex')}  ${name}\n`);
  };
  try {
    write(archive, 'archive fixture');
    assert.throws(verify, /upload set/); // A Windows archive alone is not a release.
    write(installer, 'installer fixture');
    verify();
    fs.writeFileSync(path.join(directory, archive), 'corrupted bytes');
    assert.throws(verify, /Checksum mismatch/);
    write(archive, 'archive fixture');
    write(installer, '');
    assert.throws(verify, /Empty artifact/);
    write(installer, 'installer fixture');
    fs.writeFileSync(path.join(directory, `${installer}.sha256`), fs.readFileSync(path.join(directory, `${archive}.sha256`)));
    assert.throws(verify, /Checksum mismatch/);
    write(installer, 'installer fixture');
    fs.writeFileSync(path.join(directory, 'webmux-old-native.zip'), 'stale');
    assert.throws(verify, /upload set/);
    fs.unlinkSync(path.join(directory, 'webmux-old-native.zip'));
    fs.writeFileSync(path.join(directory, 'legacy.zip'), 'unrelated legacy artifact');
    verify();
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
