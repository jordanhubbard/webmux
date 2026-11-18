import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { inspectPE, requireARM64Image, requireNativeARM64 } from './windows-native-architecture.mts';

function fixture(machine = 0xaa64): Buffer {
  const bytes = Buffer.alloc(256);
  bytes.write('MZ'); bytes.writeUInt32LE(128, 0x3c); bytes.write('PE\0\0', 128); bytes.writeUInt16LE(machine, 132);
  return bytes;
}

test('native admission refuses cross targets, emulated Node and wrong OS', () => {
  const good = { platform: 'win32', arch: 'arm64', go: { GOOS: 'windows', GOARCH: 'arm64', GOHOSTARCH: 'arm64' } };
  assert.doesNotThrow(() => requireNativeARM64(good));
  for (const value of [{ ...good, platform: 'darwin' }, { ...good, arch: 'x64' },
    ...['GOOS', 'GOARCH', 'GOHOSTARCH'].map(key => ({ ...good, go: { ...good.go, [key]: 'amd64' } }))]) {
    assert.throws(() => requireNativeARM64(value), /native Windows ARM64/);
  }
});

test('image admission hashes actual bytes and rejects AMD64 or malformed fixtures without executing them', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'wm-pe-'));
  try {
    const file = path.join(root, 'fixture.exe'), arm = fixture();
    await writeFile(file, arm);
    assert.deepEqual(await requireARM64Image(file), { machine: 'arm64', sha256: createHash('sha256').update(arm).digest('hex') });
    await writeFile(file, fixture(0x8664));
    assert.equal((await inspectPE(file)).machine, 'amd64');
    await assert.rejects(requireARM64Image(file), /Expected ARM64 image, observed amd64/);
    await writeFile(file, fixture(0x14c));
    await assert.rejects(requireARM64Image(file), /observed ia32/);
    const badDOS = fixture(); badDOS[0] = 0;
    const badOffset = fixture(); badOffset.writeUInt32LE(0xffffffff, 0x3c);
    const lowOffset = fixture(); lowOffset.writeUInt32LE(2, 0x3c);
    const badSignature = fixture(); badSignature[128] = 0;
    for (const bytes of [Buffer.alloc(12), badDOS, badOffset, lowOffset, badSignature, fixture(0xffff)]) {
      await writeFile(file, bytes); await assert.rejects(inspectPE(file));
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});
