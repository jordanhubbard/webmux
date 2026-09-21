#!/usr/bin/env node
// Build a native runtime from an explicit allowlist, never operator state.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';

const repo = path.resolve(import.meta.dirname, '..');
const source = path.join(repo, 'webmux');
const output = path.resolve(process.argv[2] || path.join(repo, 'dist'));
const metadata: unknown = JSON.parse(fs.readFileSync(path.join(source, 'package.json'), 'utf8'));
if (!metadata || typeof metadata !== 'object' || !('version' in metadata) || typeof metadata.version !== 'string'
  || !/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(metadata.version)) throw new Error('Invalid package version');
const goos = process.platform === 'win32' ? 'windows' : process.platform;
const goarch = process.arch === 'x64' ? 'amd64' : process.arch;
if (!['darwin', 'linux', 'windows'].includes(goos) || !['amd64', 'arm64'].includes(goarch)) {
  throw new Error('Native packages support macOS, Linux and Windows on x64/arm64');
}
const name = `webmux-${metadata.version}-${goos}-${process.arch}-native`;
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-native-package-'));
const stage = path.join(temporary, name);
try {
  fs.mkdirSync(path.join(stage, 'bin'), { recursive: true });
  // Reject links rather than accidentally shipping a developer's linked files.
  function copyTree(from: string, to: string): void {
    const stat = fs.lstatSync(from);
    if (stat.isDirectory()) {
      fs.mkdirSync(to, { recursive: true });
      for (const entry of fs.readdirSync(from)) copyTree(path.join(from, entry), path.join(to, entry));
    } else if (stat.isFile()) fs.copyFileSync(from, to);
    else throw new Error(`Cannot package non-regular entry: ${from}`);
  }
  fs.copyFileSync(path.join(repo, 'LICENSE'), path.join(stage, 'LICENSE'));
  if (goos === 'windows') {
    fs.mkdirSync(path.join(stage, 'service'));
    for (const entry of ['windows-service.ps1', 'runtime.psm1']) {
      copyTree(path.join(source, 'service', entry), path.join(stage, 'service', entry));
    }
    fs.writeFileSync(path.join(stage, 'bin/webmux-service.cmd'), `@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0..\\service\\windows-service.ps1" %*\r\nexit /b %ERRORLEVEL%\r\n`);
  }
  const executable = path.join(stage, 'bin', goos === 'windows' ? 'webmux.exe' : 'webmux');
  execFileSync('go', ['run', './cmd/build', '-goos', goos, '-goarch', goarch, '-o', executable], {
    cwd: path.join(source, 'server'), stdio: 'inherit',
  });
  fs.writeFileSync(path.join(stage, 'bundle.json'), JSON.stringify({
    version: metadata.version, backend: 'go', platform: process.platform, arch: process.arch,
    go: execFileSync('go', ['version'], { encoding: 'utf8' }).trim(),
  }, null, 2) + '\n');
  fs.writeFileSync(path.join(stage, 'README.txt'), `WebMux ${metadata.version} — native server

Run bin/${goos === 'windows' ? 'webmux.exe' : 'webmux'} and open http://localhost:8080.
The executable embeds the browser UI and configuration defaults.
No Node.js runtime or adjacent asset directories are required; you may copy the
executable anywhere. Optional Windows service support stays in this archive.
Configuration/state default to ~/.config/webmux; WEBMUX_HOME overrides this.
WEBMUX_ROOT or --root optionally supplies external web/ and config.defaults/
directories. Missing directories use embedded assets.
Never run multiple servers against the same writable home.
Reinstall or reconfigure services that still point to an older Node installation.
${goos === 'windows' ? 'Service installation: from an elevated terminal, run bin\\webmux-service.cmd install\nwith -WebMuxHome pointing to isolated state. The existing account prompt and\noptional -LocalSystem switch apply; no Node.js runtime is needed.\n' : ''}
OpenSSH is required for SSH; mosh, sshpass and tmux depend on selected features.
RDP requires an operator-managed guacd. These external tools are not bundled.
To upgrade, stop the server, extract a new bundle separately, then restart it
with the same WEBMUX_HOME. Do not overwrite runtime configuration with defaults.
Documentation: https://github.com/jordanhubbard/webmux
`);
  fs.mkdirSync(output, { recursive: true });
  const archive = path.join(output, `${name}.${goos === 'windows' ? 'zip' : 'tar.gz'}`);
  execFileSync('tar', goos === 'windows' ? ['-a', '-cf', archive, '-C', temporary, name]
    : ['-czf', archive, '-C', temporary, name], { stdio: 'inherit' });
  const checksum = createHash('sha256').update(fs.readFileSync(archive)).digest('hex');
  fs.writeFileSync(`${archive}.sha256`, `${checksum}  ${path.basename(archive)}\n`);
  console.log(archive);
} finally { fs.rmSync(temporary, { recursive: true, force: true }); }
