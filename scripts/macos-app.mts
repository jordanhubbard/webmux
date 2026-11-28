#!/usr/bin/env node
// Assemble a stable app identity around the same embedded native server.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const bundleID = 'com.webmux.server';
export const entitlementsPath = path.join(import.meta.dirname, '..', 'packaging', 'macos', 'WebMux.entitlements');
export const missingStableSignatureCredential = [
  'Missing credential: a Developer ID Application certificate for team ULQA4V44V5,',
  'passed as WEBMUX_MACOS_SIGN_IDENTITY. Notarization also needs the notarytool',
  'keychain profile "webmux" as WEBMUX_MACOS_NOTARY_PROFILE. That private key and',
  'profile are not in this repository. An SSH session keeps the login keychain',
  'locked, so it cannot use the key. An ad-hoc signature (codesign identity "-")',
  'is a new TCC identity on every rebuild and does not keep grants.',
].join(' ');

export type MacSignatureKind = 'unsigned' | 'adhoc' | 'stable';

export function classifyMacSignature(display: string): MacSignatureKind {
  const text = display.trim();
  if (!text || /not signed at all/i.test(text) || /code object is not signed/i.test(text)) return 'unsigned';
  if (/Signature=adhoc\b/.test(text) || /linker-signed/.test(text) || /flags=[^\n]*\badhoc\b/.test(text)) return 'adhoc';
  if (/^Authority=/m.test(text)) return 'stable';
  if (/^TeamIdentifier=(?!not set\b)\S+/m.test(text)) return 'stable';
  return 'unsigned';
}

export function cdhashes(display: string): string[] {
  return [...display.matchAll(/^CDHash=(\S+)/gm)].map((match) => match[1]).sort();
}

export function assertCDHashUnchanged(first: string, second: string): void {
  const before = cdhashes(first);
  const after = cdhashes(second);
  if (before.length === 0 || after.length === 0) {
    throw new Error('codesign did not report a CDHash. The app is not a signed release artifact.');
  }
  if (before.join(',') !== after.join(',')) {
    throw new Error('CDHash changed between read-only checks. Launch and verification must not re-sign the app.');
  }
}

export function assertReleaseSignature(display: string): void {
  const kind = classifyMacSignature(display);
  if (kind !== 'stable') throw new Error(`WebMux.app signature is ${kind}. ${missingStableSignatureCredential}`);
}

// Blank means the development identity "-". Never downgrade a certificate-backed app to it.
export function replacementIdentity(existingDisplay: string | null, requested: string | undefined): string {
  const identity = (requested ?? '').trim() || '-';
  if (existingDisplay && classifyMacSignature(existingDisplay) === 'stable' && identity === '-') {
    throw new Error(`Refusing to replace a stable WebMux.app signature with an ad-hoc signature. ${missingStableSignatureCredential}`);
  }
  return identity;
}

export function signatureDisplayArgs(target: string): string[] {
  return ['--display', '--verbose=4', target];
}

export function assertDisplayArgsAreReadOnly(args: string[]): void {
  const forbidden = new Set(['--sign', '--force', '-s', '-f', '--entitlements', '--remove-signature']);
  if (args.some((arg) => forbidden.has(arg))) throw new Error('A signature check must only display the existing signature.');
}

export function codesignSignArgs(target: string, identity: string, entitlements: string): string[] {
  const stable = identity !== '-';
  return ['--force', '--options', 'runtime', stable ? '--timestamp' : '--timestamp=none', '--entitlements', entitlements, '--sign', identity, target];
}

export function infoPlist(version: string): string {
  if (!/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version)) throw new Error('Invalid app version');
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>${bundleID}</string>
<key>CFBundleName</key><string>WebMux</string>
<key>CFBundleDisplayName</key><string>WebMux</string>
<key>CFBundleExecutable</key><string>webmux</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>${version}</string>
<key>CFBundleVersion</key><string>${version.split('-')[0]}</string>
<key>LSUIElement</key><true/>
<key>NSDesktopFolderUsageDescription</key><string>WebMux terminal sessions need access to files you work with on your Desktop.</string>
<key>NSDocumentsFolderUsageDescription</key><string>WebMux terminal sessions need access to your Documents.</string>
<key>NSDownloadsFolderUsageDescription</key><string>WebMux terminal sessions need access to files in Downloads.</string>
</dict></plist>
`;
}

function signatureDisplay(target: string): string {
  const args = signatureDisplayArgs(target);
  assertDisplayArgsAreReadOnly(args);
  const result = spawnSync('/usr/bin/codesign', args, { encoding: 'utf8' });
  return `${result.stdout ?? ''}\n${result.stderr ?? ''}`;
}

export function assembleApp(binary: string, destination: string, version: string): void {
  if (process.platform !== 'darwin') throw new Error('App assembly requires macOS');
  if (path.basename(destination) !== 'WebMux.app') throw new Error('Destination must be named WebMux.app');
  if (!fs.existsSync(entitlementsPath)) throw new Error(`Missing entitlements file: ${entitlementsPath}`);
  const plist = infoPlist(version);
  const parent = path.dirname(destination);
  fs.mkdirSync(parent, { recursive: true });
  let existingDisplay: string | null = null;
  if (fs.existsSync(destination)) {
    if (fs.lstatSync(destination).isSymbolicLink()) throw new Error('Refusing to replace a linked app');
    const existing = execFileSync('/usr/bin/plutil', ['-extract', 'CFBundleIdentifier', 'raw', path.join(destination, 'Contents/Info.plist')], { encoding: 'utf8' }).trim();
    if (existing !== bundleID) throw new Error('Refusing to replace an unrelated app');
    existingDisplay = signatureDisplay(destination);
  }
  const identity = replacementIdentity(existingDisplay, process.env.WEBMUX_MACOS_SIGN_IDENTITY);
  const temporary = fs.mkdtempSync(path.join(parent, '.webmux-app-'));
  const staged = path.join(temporary, 'WebMux.app');
  const old = path.join(temporary, 'previous.app');
  let retainBackup = false;
  try {
    fs.mkdirSync(path.join(staged, 'Contents/MacOS'), { recursive: true });
    fs.copyFileSync(binary, path.join(staged, 'Contents/MacOS/webmux'));
    fs.chmodSync(path.join(staged, 'Contents/MacOS/webmux'), 0o755);
    fs.writeFileSync(path.join(staged, 'Contents/Info.plist'), plist);
    execFileSync('/usr/bin/codesign', codesignSignArgs(staged, identity, entitlementsPath), { stdio: 'inherit', timeout: 60000 });
    execFileSync('/usr/bin/codesign', ['--verify', '--deep', '--strict', staged], { stdio: 'inherit' });
    const profile = process.env.WEBMUX_MACOS_NOTARY_PROFILE;
    if (profile) {
      if (identity === '-') throw new Error('Notarization requires WEBMUX_MACOS_SIGN_IDENTITY');
      const archive = path.join(temporary, 'WebMux.zip');
      execFileSync('/usr/bin/ditto', ['-c', '-k', '--keepParent', staged, archive]);
      execFileSync('/usr/bin/xcrun', ['notarytool', 'submit', archive, '--keychain-profile', profile, '--wait', '--timeout', '10m'], { stdio: 'inherit', timeout: 660000 });
      execFileSync('/usr/bin/xcrun', ['stapler', 'staple', staged], { stdio: 'inherit', timeout: 60000 });
      execFileSync('/usr/sbin/spctl', ['--assess', '--type', 'execute', staged], { stdio: 'inherit' });
    }
    const signed = signatureDisplay(staged);
    if (process.env.WEBMUX_MACOS_REQUIRE_STABLE_SIGN === '1') assertReleaseSignature(signed);
    // Packaging signs once. A second display must see the same CDHash.
    assertCDHashUnchanged(signed, signatureDisplay(staged));
    if (fs.existsSync(destination)) fs.renameSync(destination, old);
    try { fs.renameSync(staged, destination); }
    catch (err) {
      if (fs.existsSync(old)) {
        try { fs.renameSync(old, destination); }
        catch { retainBackup = true; throw new Error(`App replacement failed; previous app retained at ${old}`); }
      }
      throw err;
    }
    if (identity === '-') console.log('WebMux.app uses an ad-hoc development signature. Privacy grants do not stick across rebuilds. Set WEBMUX_MACOS_SIGN_IDENTITY and run make check-macos-signature. The server does not sign again at launch.');
    else console.log('WebMux.app signed with a stable identity. Starting the service does not re-sign. Local PTYs inherit this app\'s TCC responsibility; SSH sessions do not.');
    console.log(`Installed ${destination}`);
  } finally { if (!retainBackup) fs.rmSync(temporary, { recursive: true, force: true }); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const repo = path.resolve(import.meta.dirname, '..');
  const { version } = JSON.parse(fs.readFileSync(path.join(repo, 'webmux/package.json'), 'utf8')) as { version: string };
  assembleApp(path.resolve(process.argv[2] ?? path.join(repo, 'webmux/bin/webmux')),
    path.resolve(process.argv[3] ?? path.join(os.homedir(), 'Applications/WebMux.app')), version);
}
