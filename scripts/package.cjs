#!/usr/bin/env node
// Stage an installable runtime without copying local configuration or node_modules.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { createHash } = require('node:crypto');

const repo = path.resolve(__dirname, '..');
const source = path.join(repo, 'webmux');
const output = path.resolve(process.argv[2] || path.join(repo, 'dist'));
const version = JSON.parse(fs.readFileSync(path.join(source, 'backend/package.json'))).version;
if (!['darwin', 'linux', 'win32'].includes(process.platform) || process.versions.node.split('.')[0] !== '24') {
  throw new Error('Build release bundles on macOS, Linux, or Windows using Node.js 24.');
}
const platformName = process.platform === 'win32' ? 'windows' : process.platform;
const name = `webmux-${version}-${platformName}-${process.arch}-node24`;
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-package-'));
const stage = path.join(temporary, name);
fs.mkdirSync(stage);
try {
  // The caller builds first. Explicit allowlist excludes credentials and development tools.
  const entries = ['package.json', 'package-lock.json', 'backend/package.json',
    'backend/dist', 'frontend/package.json', 'web', 'config.defaults'];
  if (process.platform === 'win32') entries.push('service/windows-service.ps1');
  for (const entry of entries) {
    const destination = path.join(stage, entry);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.cpSync(path.join(source, entry), destination, { recursive: true });
  }
  fs.copyFileSync(path.join(repo, 'LICENSE'), path.join(stage, 'LICENSE'));
  execFileSync('npm', ['ci', '--omit=dev', '--workspace=backend', '--no-audit', '--no-fund'], {
    cwd: stage, stdio: 'inherit',
  });
  fs.mkdirSync(path.join(stage, 'bin'));
  if (process.platform === 'win32') {
    fs.writeFileSync(path.join(stage, 'bin/webmux.js'), `const path = require('node:path');
if (process.versions.node.split('.')[0] !== '24') {
  console.error('This WebMux bundle requires Node.js 24 on PATH.');
  process.exit(1);
}
const root = path.resolve(__dirname, '..');
process.env.WEBMUX_ROOT = root;
require(path.join(root, 'backend/dist/index.js'));
`);
    fs.writeFileSync(path.join(stage, 'bin/webmux.cmd'), `@echo off\r
node "%~dp0webmux.js" %*\r
exit /b %ERRORLEVEL%\r
`);
    fs.writeFileSync(path.join(stage, 'bin/webmux-service.cmd'), `@echo off\r
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0..\\service\\windows-service.ps1" %*\r
exit /b %ERRORLEVEL%\r
`);
  } else {
    fs.writeFileSync(path.join(stage, 'bin/webmux'), `#!/bin/sh
set -eu
WEBMUX_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export WEBMUX_ROOT
if [ "$(node -p 'process.versions.node.split(".")[0]')" != 24 ]; then
  echo "This WebMux bundle requires Node.js 24 on PATH." >&2
  exit 1
fi
exec node "$WEBMUX_ROOT/backend/dist/index.js" "$@"
`, { mode: 0o755 });
  }
  fs.writeFileSync(path.join(stage, 'bundle.json'), JSON.stringify({
    version, platform: process.platform, arch: process.arch, node: process.versions.node,
    nodeABI: process.versions.modules,
    glibc: process.report.getReport().header.glibcVersionRuntime,
  }, null, 2) + '\n');
  fs.writeFileSync(path.join(stage, 'README.txt'), process.platform === 'win32' ? `WebMux ${version}

Requires Node.js 24 and Microsoft OpenSSH Client on PATH.
Run bin\\webmux.cmd, then open http://localhost:8080.
Configuration and state: %USERPROFILE%\\.config\\webmux, overridden by WEBMUX_HOME.
From an elevated terminal, bin\\webmux-service.cmd install registers the Windows service.
To upgrade, stop WebMux and install the newer MSI or extract the new bundle separately.
Runtime data is kept outside this directory and is preserved across upgrades.
Documentation: https://github.com/jordanhubbard/webmux
` : `WebMux ${version}

Requires Node.js 24 and OpenSSH on PATH, and the OS/CPU listed in bundle.json.
Extract the whole archive, then run ./bin/webmux from the extracted directory.
Do not move or symlink bin/webmux separately from the rest of the bundle.
Open http://localhost:8080 and create the first administrator account.
Configuration and state: ~/.config/webmux, overridden by WEBMUX_HOME.
To upgrade, stop WebMux and extract the new bundle into a separate directory.
Keep WEBMUX_HOME unchanged and start the new bundle. Never extract over an old bundle.
Optional commands: mosh, sshpass, tmux; RDP requires guacd.
For managed services on macOS or Linux, use the Homebrew package instead.
Documentation: https://github.com/jordanhubbard/webmux
`);
  fs.mkdirSync(output, { recursive: true });
  const archive = path.join(output, `${name}.${process.platform === 'win32' ? 'zip' : 'tar.gz'}`);
  const archiveArgs = process.platform === 'win32'
    ? ['-a', '-cf', archive, '-C', temporary, name]
    : ['-czf', archive, '-C', temporary, name];
  execFileSync('tar', archiveArgs, { stdio: 'inherit' });
  const checksum = createHash('sha256').update(fs.readFileSync(archive)).digest('hex');
  fs.writeFileSync(`${archive}.sha256`, `${checksum}  ${path.basename(archive)}\n`);
  console.log(archive);
} finally {
  fs.rmSync(temporary, { recursive: true, force: true });
}
