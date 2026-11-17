#!/usr/bin/env node
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repository = 'https://github.com/jordanhubbard/webmux';
const identifier = 'WebMux.WebMux';
const schema = '1.28.0';
function object(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value), 'Expected metadata object');
  return value as Record<string, unknown>;
}
function guid(value: unknown): string {
  assert(typeof value === 'string' && /^\{[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\}$/i.test(value), 'Invalid MSI product code');
  return value.toUpperCase();
}
const quote = (value: string) => JSON.stringify(value);
const header = (kind: string) => `# yaml-language-server: $schema=https://aka.ms/winget-manifest.${kind}.1.28.0.schema.json\n`;

/** Bind offline MSI properties to exact bytes and published GitHub asset digests. */
export function generateWinget(releaseValue: unknown, inspectionValue: unknown, artifacts: string): Record<string, string> {
  const release = object(releaseValue), inspection = object(inspectionValue);
  assert(typeof release.tag_name === 'string' && /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(release.tag_name), 'Expected stable release tag');
  const tag = release.tag_name, version = tag.slice(1);
  assert.equal(release.html_url, `${repository}/releases/tag/${tag}`, 'Unexpected release repository');
  assert.equal(release.draft, false, 'Draft release');
  assert.equal(release.prerelease, false, 'Prerelease');
  assert(typeof release.published_at === 'string' && Number.isFinite(Date.parse(release.published_at)), 'Unpublished release');
  assert.equal(inspection.version, version, 'MSI inspection version mismatch');
  assert(Array.isArray(release.assets) && Array.isArray(inspection.installers), 'Missing release/MSI assets');
  assert.equal(inspection.installers.length, 2, 'Expected exactly two MSI inspections');
  const installers: string[] = [], products = new Set<string>();
  for (const architecture of ['x64', 'arm64']) {
    const name = `webmux-${version}-windows-${architecture}-native.msi`;
    const url = `${repository}/releases/download/${tag}/${name}`;
    const assets: Record<string, unknown>[] = release.assets.map(object).filter(asset => asset.name === name);
    const records: Record<string, unknown>[] = inspection.installers.map(object).filter(record => record.architecture === architecture);
    assert.equal(assets.length, 1, `Missing or duplicate release MSI: ${architecture}`);
    assert.equal(records.length, 1, `Missing or duplicate MSI inspection: ${architecture}`);
    const asset = assets[0], record = records[0], properties = object(record.properties);
    assert.equal(asset.browser_download_url, url, 'Unexpected release asset URL');
    assert.equal(record.name, name, 'MSI filename mismatch');
    assert.equal(record.url, url, 'MSI inspection URL mismatch');
    const file = path.join(artifacts, name);
    assert(fs.lstatSync(file).isFile(), 'MSI must be a regular file');
    const bytes = fs.readFileSync(file);
    assert(bytes.length > 0 && asset.size === bytes.length, 'Release asset size mismatch');
    const digest = createHash('sha256').update(bytes).digest('hex');
    assert.equal(asset.digest, `sha256:${digest}`, 'GitHub asset digest mismatch');
    assert.equal(record.sha256, digest, 'MSI inspection digest mismatch');
    assert.equal(fs.readFileSync(`${file}.sha256`, 'utf8').trim(), `${digest}  ${name}`, 'Release checksum mismatch');
    assert.equal(properties.ProductName, 'WebMux', 'Unexpected MSI product');
    assert.equal(properties.Manufacturer, 'WebMux', 'Unexpected MSI publisher');
    assert.equal(properties.ProductVersion, version, 'MSI product version mismatch');
    assert.equal(guid(properties.UpgradeCode), '{1E20A25F-F940-4B0B-8AC0-7E785279F7C5}', 'Unexpected MSI upgrade family');
    assert(properties.ALLUSERS === null && properties.MSIINSTALLPERUSER === null, 'Recheck per-user MSI scope before generation');
    assert.equal(record.template, architecture === 'x64' ? 'x64;1033' : 'Arm64;1033', 'MSI architecture mismatch');
    const product = guid(properties.ProductCode);
    assert(!products.has(product), 'Architectures must have different MSI product codes'); products.add(product);
    installers.push(`- Architecture: ${architecture}\n  InstallerUrl: ${quote(url)}\n  InstallerSha256: ${digest.toUpperCase()}\n  ProductCode: ${quote(product)}\n  AppsAndFeaturesEntries:\n  - DisplayName: WebMux\n    Publisher: WebMux\n    DisplayVersion: ${quote(version)}\n    ProductCode: ${quote(product)}\n    UpgradeCode: "{1E20A25F-F940-4B0B-8AC0-7E785279F7C5}"`);
  }
  const identity = `PackageIdentifier: ${identifier}\nPackageVersion: ${quote(version)}\n`;
  return {
    [`${identifier}.yaml`]: header('version') + identity + `DefaultLocale: en-US\nManifestType: version\nManifestVersion: ${schema}\n`,
    [`${identifier}.locale.en-US.yaml`]: header('defaultLocale') + identity + `PackageLocale: en-US\nPublisher: WebMux\nPublisherUrl: ${repository}\nPublisherSupportUrl: ${repository}/issues\nPackageName: WebMux\nPackageUrl: ${repository}\nLicense: BSD-2-Clause\nLicenseUrl: ${repository}/blob/${tag}/LICENSE\nShortDescription: A web terminal manager with persistent sessions and remote host access.\nMoniker: webmux\nTags:\n- terminal\n- ssh\n- remote-access\nReleaseNotesUrl: ${repository}/releases/tag/${tag}\nManifestType: defaultLocale\nManifestVersion: ${schema}\n`,
    [`${identifier}.installer.yaml`]: header('installer') + identity + `InstallerType: wix\nScope: user\nInstallModes:\n- silent\n- silentWithProgress\nUpgradeBehavior: install\nCommands:\n- webmux\n- webmux-client\nInstallers:\n${installers.join('\n')}\nManifestType: installer\nManifestVersion: ${schema}\n`,
  };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [releaseFile, inspectionFile, artifacts, outputRoot, ...extra] = process.argv.slice(2);
  assert(releaseFile && inspectionFile && artifacts && outputRoot && !extra.length,
    'Usage: node scripts/generate-winget.mts <release.json> <msi-inspection.json> <artifact-directory> <output-root>');
  const release: unknown = JSON.parse(fs.readFileSync(releaseFile, 'utf8'));
  const files = generateWinget(release, JSON.parse(fs.readFileSync(inspectionFile, 'utf8')) as unknown, artifacts);
  const tag = object(release).tag_name as string;
  const directory = path.resolve(outputRoot, 'manifests/w/WebMux/WebMux', tag.slice(1));
  assert(!fs.existsSync(directory), 'Output version already exists; review/remove it explicitly before regenerating');
  fs.mkdirSync(directory, { recursive: true });
  for (const [name, contents] of Object.entries(files)) fs.writeFileSync(path.join(directory, name), contents, { flag: 'wx' });
  console.log(`Generated ${directory}; validate with winget and submit through an upstream PR. Publication is separate.`);
}
