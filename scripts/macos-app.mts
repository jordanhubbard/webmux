#!/usr/bin/env node
// Assemble a stable app identity around the same embedded native server.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const bundleID = 'com.webmux.server';
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
export function assembleApp(binary: string, destination: string, version: string): void {
  if (process.platform !== 'darwin') throw new Error('App assembly requires macOS');
  if (path.basename(destination) !== 'WebMux.app') throw new Error('Destination must be named WebMux.app');
  const plist = infoPlist(version);
  const parent = path.dirname(destination);
  fs.mkdirSync(parent, { recursive: true });
  if (fs.existsSync(destination)) {
    if (fs.lstatSync(destination).isSymbolicLink()) throw new Error('Refusing to replace a linked app');
    const existing = execFileSync('/usr/bin/plutil', ['-extract', 'CFBundleIdentifier', 'raw', path.join(destination, 'Contents/Info.plist')], { encoding: 'utf8' }).trim();
    if (existing !== bundleID) throw new Error('Refusing to replace an unrelated app');
  }
  const temporary = fs.mkdtempSync(path.join(parent, '.webmux-app-'));
  const staged = path.join(temporary, 'WebMux.app');
  const old = path.join(temporary, 'previous.app');
  try {
    fs.mkdirSync(path.join(staged, 'Contents/MacOS'), { recursive: true });
    fs.copyFileSync(binary, path.join(staged, 'Contents/MacOS/webmux'));
    fs.chmodSync(path.join(staged, 'Contents/MacOS/webmux'), 0o755);
    fs.writeFileSync(path.join(staged, 'Contents/Info.plist'), plist);
    const identity = process.env.WEBMUX_MACOS_SIGN_IDENTITY || '-';
    execFileSync('/usr/bin/codesign', ['--force', '--options', 'runtime', ...(identity === '-' ? ['--timestamp=none'] : ['--timestamp']), '--sign', identity, staged], { stdio: 'inherit', timeout: 60000 });
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
    if (fs.existsSync(destination)) fs.renameSync(destination, old);
    try { fs.renameSync(staged, destination); }
    catch (err) { if (fs.existsSync(old)) fs.renameSync(old, destination); throw err; }
    if (identity === '-') console.log('WebMux.app uses a development signature. Release distribution requires Developer ID signing and notarization. Recheck permissions after rebuilding.');
    console.log(`Installed ${destination}`);
  } finally { fs.rmSync(temporary, { recursive: true, force: true }); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const repo = path.resolve(import.meta.dirname, '..');
  const { version } = JSON.parse(fs.readFileSync(path.join(repo, 'webmux/package.json'), 'utf8')) as { version: string };
  assembleApp(path.resolve(process.argv[2] ?? path.join(repo, 'webmux/bin/webmux')),
    path.resolve(process.argv[3] ?? path.join(os.homedir(), 'Applications/WebMux.app')), version);
}
