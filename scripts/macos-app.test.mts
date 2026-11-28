import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import {
  assertCDHashUnchanged,
  assertDisplayArgsAreReadOnly,
  assertReleaseSignature,
  classifyMacSignature,
  codesignSignArgs,
  entitlementsPath,
  infoPlist,
  missingStableSignatureCredential,
  replacementIdentity,
  signatureDisplayArgs,
} from './macos-app.mts';

const stable = [
  'Authority=Developer ID Application: Example (ULQA4V44V5)',
  'Authority=Developer ID Certification Authority',
  'Authority=Apple Root CA',
  'TeamIdentifier=ULQA4V44V5',
  'CDHash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  'CDHash=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
].join('\n');
const adhoc = [
  'Signature=adhoc',
  'flags=0x10002(adhoc,runtime)',
  'TeamIdentifier=not set',
  'CDHash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
].join('\n');
const linker = 'flags=0x20002(adhoc,linker-signed)\nSignature=adhoc\n';
const unsigned = '/Applications/WebMux.app: code object is not signed at all\n';

test('app identity is stable across upgrades and explains protected folder access', () => {
  for (const version of ['1.3.20', '1.3.21']) {
    const plist = infoPlist(version);
    assert(plist.includes('<string>com.webmux.server</string>'));
    assert(plist.includes('<key>CFBundleExecutable</key><string>webmux</string>'));
    for (const folder of ['Desktop', 'Documents', 'Downloads']) assert(plist.includes(`NS${folder}FolderUsageDescription`));
  }
  assert.throws(() => infoPlist('1.0.0</string>'), /Invalid/);
});

test('release signature rejects unsigned and ad-hoc identities', () => {
  assert.equal(classifyMacSignature(stable), 'stable');
  assert.equal(classifyMacSignature(adhoc), 'adhoc');
  assert.equal(classifyMacSignature(linker), 'adhoc');
  assert.equal(classifyMacSignature(unsigned), 'unsigned');
  assert.equal(classifyMacSignature(''), 'unsigned');
  assert.equal(classifyMacSignature('Authority=WebMux Local\nTeamIdentifier=not set\nCDHash=abc\n'), 'stable');
  assert.doesNotThrow(() => assertReleaseSignature(stable));
  assert.throws(() => assertReleaseSignature(adhoc), /signature is adhoc/);
  assert.throws(() => assertReleaseSignature(unsigned), /signature is unsigned/);
  assert.match(missingStableSignatureCredential, /ULQA4V44V5/);
  assert.match(missingStableSignatureCredential, /WEBMUX_MACOS_NOTARY_PROFILE/);
});

test('a stable app is not replaced by an ad-hoc signature', () => {
  assert.throws(() => replacementIdentity(stable, undefined), /ad-hoc/);
  assert.throws(() => replacementIdentity(stable, '  '), /ad-hoc/);
  assert.equal(replacementIdentity(adhoc, undefined), '-');
  assert.equal(replacementIdentity(unsigned, ''), '-');
  assert.equal(replacementIdentity(null, 'Developer ID Application: Example (ULQA4V44V5)'), 'Developer ID Application: Example (ULQA4V44V5)');
});

test('signature checks are read-only and notice a re-sign', () => {
  const display = signatureDisplayArgs('/Applications/WebMux.app');
  assert.deepEqual(display, ['--display', '--verbose=4', '/Applications/WebMux.app']);
  assert.doesNotThrow(() => assertDisplayArgsAreReadOnly(display));
  assert.throws(() => assertDisplayArgsAreReadOnly(['--force', '--sign', '-', '/Applications/WebMux.app']), /display/);
  assert.doesNotThrow(() => assertCDHashUnchanged(stable, stable));
  assert.throws(() => assertCDHashUnchanged(stable, stable.replaceAll('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'cccccccccccccccccccccccccccccccccccccccc')), /CDHash changed/);
  assert.throws(() => assertCDHashUnchanged('no hash', 'no hash'), /CDHash/);
  assert.deepEqual(codesignSignArgs('/tmp/WebMux.app', '-', entitlementsPath), [
    '--force', '--options', 'runtime', '--timestamp=none', '--entitlements', entitlementsPath, '--sign', '-', '/tmp/WebMux.app',
  ]);
  const identity = 'Developer ID Application: Example (ULQA4V44V5)';
  assert.deepEqual(codesignSignArgs('/tmp/WebMux.app', identity, entitlementsPath), [
    '--force', '--options', 'runtime', '--timestamp', '--entitlements', entitlementsPath, '--sign', identity, '/tmp/WebMux.app',
  ]);
});

test('entitlements do not weaken the hardened runtime', () => {
  const plist = fs.readFileSync(entitlementsPath, 'utf8');
  assert.match(plist, /<plist version="1.0">/);
  assert.doesNotMatch(plist, /get-task-allow|disable-library-validation|keychain-access-groups/);
});

test('the launch path and signature check do not sign', () => {
  const checker = fs.readFileSync(new URL('./check-macos-signature.mts', import.meta.url), 'utf8');
  assert.doesNotMatch(checker, /--sign\b|--force\b|assembleApp/);
  const repo = path.resolve(import.meta.dirname, '..');
  const roots = [
    path.join(repo, 'webmux/server/cmd'),
    path.join(repo, 'webmux/server/internal/macsetup'),
    path.join(repo, 'webmux/server/internal/terminal'),
  ];
  const hits: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile() && entry.name.endsWith('.go') && fs.readFileSync(full, 'utf8').includes('codesign')) hits.push(full);
    }
  };
  for (const root of roots) walk(root);
  assert.deepEqual(hits, []);
  const formula = fs.readFileSync(path.join(repo, 'Formula/webmux.rb'), 'utf8');
  assert.match(formula, /run macos: \[opt_libexec\/"WebMux\.app\/Contents\/MacOS\/webmux"\]/);
  assert.match(formula, /linux: \[opt_bin\/"webmux"\]/);
});
