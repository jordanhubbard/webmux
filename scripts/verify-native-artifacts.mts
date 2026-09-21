import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** Verify the complete native upload set; installed-runtime smoke checks its contents. */
export function verifyNativeArtifacts(directory: string, version: string, platform: string, arch: string): void {
  assert(/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version), 'Invalid native version');
  assert(['darwin', 'linux', 'win32'].includes(platform), 'Unsupported native platform');
  assert(['x64', 'arm64'].includes(arch), 'Unsupported native architecture');
  const stem = `webmux-${version}-${platform === 'win32' ? 'windows' : platform}-${arch}-native`;
  const artifacts = (platform === 'win32' ? ['zip', 'msi'] : ['tar.gz']).map(extension => `${stem}.${extension}`);
  const expected = artifacts.flatMap(name => [name, `${name}.sha256`]).sort();
  const actual = fs.readdirSync(directory).filter(name => name.includes('-native.')).sort();
  assert.deepEqual(actual, expected, 'Native upload set must contain exactly this version/platform and its checksums');
  for (const name of expected) assert(fs.lstatSync(path.join(directory, name)).isFile(), `Not a regular artifact: ${name}`);
  for (const name of artifacts) {
    const bytes = fs.readFileSync(path.join(directory, name));
    assert(bytes.length > 0, `Empty artifact: ${name}`);
    const digest = createHash('sha256').update(bytes).digest('hex');
    const checksum = fs.readFileSync(path.join(directory, `${name}.sha256`), 'utf8').trim();
    assert.equal(checksum, `${digest}  ${name}`, `Checksum mismatch: ${name}`);
  }
}

/** Recheck downloaded artifacts before publication, including every supported target. */
export function verifyNativeReleaseArtifacts(directory: string, version: string): void {
  const targets = [
    ['macos-15', 'darwin', 'arm64'],
    ['ubuntu-22.04', 'linux', 'x64'],
    ['windows-2022', 'win32', 'x64'],
    ['windows-11-arm', 'win32', 'arm64'],
  ] as const;
  assert.deepEqual(fs.readdirSync(directory).sort(), targets.map(([runner]) => `native-preview-${runner}`).sort(),
    'Native release must contain exactly all four platform artifact directories');
  for (const [runner, platform, arch] of targets) {
    const root = path.join(directory, `native-preview-${runner}`);
    assert(fs.lstatSync(root).isDirectory(), `Not a regular artifact directory: ${runner}`);
    assert(fs.readdirSync(root).every(name => name.includes('-native.')), 'Unexpected non-native release file');
    verifyNativeArtifacts(root, version, platform, arch);
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = path.resolve(import.meta.dirname, '..');
  const metadata: unknown = JSON.parse(fs.readFileSync(path.join(root, 'webmux/backend/package.json'), 'utf8'));
  assert(metadata && typeof metadata === 'object' && 'version' in metadata && typeof metadata.version === 'string');
  if (process.argv[2] === '--release') {
    verifyNativeReleaseArtifacts(path.resolve(process.argv[3] ?? path.join(root, 'dist')), metadata.version);
  } else {
    verifyNativeArtifacts(path.resolve(process.argv[2] ?? path.join(root, 'dist')), metadata.version, process.platform, process.arch);
  }
  console.log('Native archive/installer upload set and SHA-256 checksums verified.');
}
