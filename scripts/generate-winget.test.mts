import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { generateWinget } from './generate-winget.mts';

function fixture(directory: string) {
  const version = '1.4.5', repository = 'https://github.com/jordanhubbard/webmux';
  const assets = ['x64', 'arm64'].map(architecture => {
    const name = `webmux-${version}-windows-${architecture}-native.msi`;
    const bytes = Buffer.from(`independent fixture ${architecture}`);
    const sha256 = createHash('sha256').update(bytes).digest('hex');
    fs.writeFileSync(path.join(directory, name), bytes);
    fs.writeFileSync(path.join(directory, `${name}.sha256`), `${sha256}  ${name}\n`);
    return { name, size: bytes.length, digest: `sha256:${sha256}`, browser_download_url: `${repository}/releases/download/v${version}/${name}` };
  });
  const release = { tag_name: `v${version}`, draft: false, prerelease: false, published_at: '2026-11-06T19:46:36Z', html_url: `${repository}/releases/tag/v${version}`, assets };
  const inspection = { version, installers: assets.map((asset, index) => ({ architecture: index ? 'arm64' : 'x64', name: asset.name, url: asset.browser_download_url,
    sha256: asset.digest.slice(7), template: index ? 'Arm64;1033' : 'x64;1033', properties: {
      ProductName: 'WebMux', Manufacturer: 'WebMux', ProductVersion: version, ALLUSERS: null as string | null, MSIINSTALLPERUSER: null,
      ProductCode: index ? '{B680C01E-C36D-4F11-B450-4D758B5257DA}' : '{5E2F88F9-A4CC-4410-A711-C63E10816201}',
      UpgradeCode: '{1E20A25F-F940-4B0B-8AC0-7E785279F7C5}',
    } })) };
  return { release, inspection };
}

test('manifests describe both verified native per-user MSI identities without a Node dependency', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-winget-'));
  try {
    const { release, inspection } = fixture(directory);
    const files = generateWinget(release, inspection, directory);
    assert.deepEqual(Object.keys(files).sort(), ['WebMux.WebMux.installer.yaml', 'WebMux.WebMux.locale.en-US.yaml', 'WebMux.WebMux.yaml']);
    const installer = files['WebMux.WebMux.installer.yaml'];
    for (const record of inspection.installers) {
      assert(installer.includes(`InstallerUrl: "${record.url}"`));
      assert(installer.includes(`InstallerSha256: ${record.sha256.toUpperCase()}`));
      assert(installer.includes(`ProductCode: "${record.properties.ProductCode}"`));
    }
    assert.match(installer, /InstallerType: wix\nScope: user/);
    assert(!installer.includes('Dependencies:'));
    assert(!installer.includes('Node'));
    assert.match(files['WebMux.WebMux.locale.en-US.yaml'], /Publisher: WebMux\n/);
    assert.deepEqual(generateWinget(release, inspection, directory), files, 'Generation must be deterministic');
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('rejects unpublished releases, wrong sources, ambiguous identities and metadata detached from MSI bytes', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-winget-'));
  try {
    const valid = fixture(directory);
    const cases: [string, (value: typeof valid) => void][] = [
      ['draft', value => { value.release.draft = true; }],
      ['prerelease', value => { value.release.prerelease = true; }],
      ['unpublished', value => { value.release.published_at = ''; }],
      ['path traversal version', value => { value.release.tag_name = 'v../../other'; }],
      ['wrong repository', value => { value.release.html_url = 'https://example.org/releases/tag/v1.4.5'; }],
      ['wrong URL', value => { value.release.assets[0].browser_download_url += '?different'; }],
      ['asset digest', value => { value.release.assets[0].digest = 'sha256:' + '0'.repeat(64); }],
      ['asset size', value => { value.release.assets[0].size++; }],
      ['inspection digest', value => { value.inspection.installers[0].sha256 = '0'.repeat(64); }],
      ['inspection URL', value => { value.inspection.installers[0].url = value.inspection.installers[1].url; }],
      ['inspection version', value => { value.inspection.version = '1.3.10'; }],
      ['MSI product', value => { value.inspection.installers[0].properties.ProductName = 'Other'; }],
      ['MSI publisher', value => { value.inspection.installers[0].properties.Manufacturer = 'Other'; }],
      ['MSI version', value => { value.inspection.installers[0].properties.ProductVersion = '1.3.10'; }],
      ['machine scope', value => { value.inspection.installers[0].properties.ALLUSERS = '1'; }],
      ['wrong architecture', value => { value.inspection.installers[0].template = 'Arm64;1033'; }],
      ['wrong upgrade family', value => { value.inspection.installers[0].properties.UpgradeCode = '{00000000-0000-0000-0000-000000000000}'; }],
      ['duplicate product', value => { value.inspection.installers[0].properties.ProductCode = value.inspection.installers[1].properties.ProductCode; }],
      ['invalid product', value => { value.inspection.installers[0].properties.ProductCode = 'bad'; }],
      ['missing architecture', value => { value.inspection.installers.pop(); }],
      ['duplicate asset', value => { value.release.assets.push(value.release.assets[0]); }],
    ];
    for (const [label, mutate] of cases) {
      const value = structuredClone(valid); mutate(value);
      assert.throws(() => generateWinget(value.release, value.inspection, directory), label);
    }
    const file = path.join(directory, valid.release.assets[0].name);
    fs.writeFileSync(`${file}.sha256`, '0'.repeat(64));
    assert.throws(() => generateWinget(valid.release, valid.inspection, directory), /checksum mismatch/);
    fixture(directory);
    fs.appendFileSync(file, 'tampered');
    assert.throws(() => generateWinget(valid.release, valid.inspection, directory), /size mismatch/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
