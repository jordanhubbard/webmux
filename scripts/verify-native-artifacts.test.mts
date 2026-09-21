import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { verifyNativeArtifacts, verifyNativeReleaseArtifacts } from './verify-native-artifacts.mts';

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
    const debug = archive.replace(/\.zip$/, '.wixpdb');
    fs.writeFileSync(path.join(directory, debug), 'installer debug database');
    assert.throws(verify, /upload set/);
    fs.unlinkSync(path.join(directory, debug));
    fs.writeFileSync(path.join(directory, 'legacy.zip'), 'unrelated legacy artifact');
    verify();
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('publication requires all native targets and rejects mixed or tampered downloads', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-native-release-'));
  const verify = () => verifyNativeReleaseArtifacts(directory, '1.2.3');
  const targets = [
    ['macos-15', 'darwin-arm64', ['tar.gz']],
    ['ubuntu-22.04', 'linux-x64', ['tar.gz']],
    ['windows-2022', 'windows-x64', ['zip', 'msi']],
    ['windows-11-arm', 'windows-arm64', ['zip', 'msi']],
  ] as const;
  try {
    for (const [runner, target, extensions] of targets) {
      assert.throws(verify, /all four/);
      const root = path.join(directory, `native-preview-${runner}`);
      fs.mkdirSync(root);
      for (const extension of extensions) {
        const name = `webmux-1.2.3-${target}-native.${extension}`;
        fs.writeFileSync(path.join(root, name), name);
        fs.writeFileSync(path.join(root, `${name}.sha256`), `${createHash('sha256').update(name).digest('hex')}  ${name}\n`);
      }
    }
    verify();
    const root = path.join(directory, 'native-preview-windows-2022');
    const extra = path.join(root, 'legacy.zip');
    fs.writeFileSync(extra, 'legacy');
    assert.throws(verify, /non-native/);
    fs.unlinkSync(extra);
    assert.throws(() => verifyNativeReleaseArtifacts(directory, '1.2.4'), /upload set/);
    const archive = path.join(root, 'webmux-1.2.3-windows-x64-native.zip');
    fs.writeFileSync(archive, 'modified after upload');
    assert.throws(verify, /Checksum mismatch/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
