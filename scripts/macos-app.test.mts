import assert from 'node:assert/strict';
import { test } from 'node:test';
import { infoPlist } from './macos-app.mts';

test('app identity is stable across upgrades and explains protected folder access', () => {
  for (const version of ['1.3.20', '1.3.21']) {
    const plist = infoPlist(version);
    assert(plist.includes('<string>com.webmux.server</string>'));
    assert(plist.includes('<key>CFBundleExecutable</key><string>webmux</string>'));
    for (const folder of ['Desktop', 'Documents', 'Downloads']) assert(plist.includes(`NS${folder}FolderUsageDescription`));
  }
  assert.throws(() => infoPlist('1.0.0</string>'), /Invalid/);
});
