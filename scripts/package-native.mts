#!/usr/bin/env node
// Build a native runtime from an explicit allowlist, never operator state.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { assembleApp } from './macos-app.mts';

const repo = path.resolve(import.meta.dirname, '..');
const source = path.join(repo, 'webmux');
const output = path.resolve(process.argv[2] || path.join(repo, 'dist'));
const metadata: unknown = JSON.parse(fs.readFileSync(path.join(source, 'package.json'), 'utf8'));
if (!metadata || typeof metadata !== 'object' || !('version' in metadata) || typeof metadata.version !== 'string'
  || !/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(metadata.version)) throw new Error('Invalid package version');
const goos = process.platform === 'win32' ? 'windows' : process.platform;
const goarch = process.arch === 'x64' ? 'amd64' : process.arch;
const component = process.env.WEBMUX_NATIVE_COMPONENT ?? 'all';
if (!['all', 'server', 'client'].includes(component)) throw new Error('WEBMUX_NATIVE_COMPONENT must be all, server or client');
if (!['darwin', 'linux', 'windows'].includes(goos) || !['amd64', 'arm64'].includes(goarch)) {
  throw new Error('Native packages support macOS, Linux and Windows on x64/arm64');
}
const prefix = component === 'all' ? 'webmux' : `webmux-${component}`;
const name = `${prefix}-${metadata.version}-${goos}-${process.arch}-native`;
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
  if (component !== 'client' && goos === 'windows') {
    fs.mkdirSync(path.join(stage, 'service'));
    for (const entry of ['windows-service.ps1', 'runtime.psm1']) {
      copyTree(path.join(source, 'service', entry), path.join(stage, 'service', entry));
    }
    fs.writeFileSync(path.join(stage, 'bin/webmux-service.cmd'), `@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0..\\service\\windows-service.ps1" %*\r\nexit /b %ERRORLEVEL%\r\n`);
  }
  let executable: string | undefined;
  let clientExecutable: string | undefined;
  if (component !== 'client') {
    executable = path.join(stage, 'bin', goos === 'windows' ? 'webmux.exe' : 'webmux');
    execFileSync('go', ['run', './cmd/build', '-goos', goos, '-goarch', goarch, '-o', executable], {
      cwd: path.join(source, 'server'), stdio: 'inherit',
    });
    if (goos === 'darwin') assembleApp(executable, path.join(stage, 'WebMux.app'), metadata.version);
  }
  if (component !== 'server') {
    clientExecutable = path.join(stage, 'bin', goos === 'windows' ? 'webmux-client.exe' : 'webmux-client');
    execFileSync('go', ['build', '-trimpath', '-o', clientExecutable, './cmd/webmux-client'], {
      cwd: path.join(source, 'server'), stdio: 'inherit',
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
    });
  }
  fs.writeFileSync(path.join(stage, 'bundle.json'), JSON.stringify({
    version: metadata.version, backend: 'go', component, platform: process.platform, arch: process.arch,
    go: execFileSync('go', ['version'], { cwd: path.join(source, 'server'), encoding: 'utf8' }).trim(),
  }, null, 2) + '\n');
  const inventory: Record<string, unknown> = {
    description: 'Native Go build identity. External SSH, mosh, tmux, guacd and native USB drivers are operator-managed and are not bundled or qualified by this inventory.',
  };
  if (executable) {
    const lock = JSON.parse(fs.readFileSync(path.join(source, 'package-lock.json'), 'utf8')) as {
      packages: Record<string, { version?: string; resolved?: string; integrity?: string; dev?: boolean; link?: boolean }>;
    };
    inventory.binarySHA256 = createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
    inventory.goBuild = execFileSync('go', ['version', '-m', executable], { cwd: path.join(source, 'server'), encoding: 'utf8' }).trim();
    inventory.browserDependencies = Object.entries(lock.packages).filter(([location, entry]) => location.includes('node_modules/') && !entry.dev && !entry.link)
      .map(([location, entry]) => ({ name: location.slice(location.lastIndexOf('node_modules/') + 13), version: entry.version, resolved: entry.resolved, integrity: entry.integrity }));
  }
  if (clientExecutable) {
    inventory.clientBinarySHA256 = createHash('sha256').update(fs.readFileSync(clientExecutable)).digest('hex');
    inventory.clientGoBuild = execFileSync('go', ['version', '-m', clientExecutable], { cwd: path.join(source, 'server'), encoding: 'utf8' }).trim();
  }
  fs.writeFileSync(path.join(stage, 'components.json'), JSON.stringify(inventory, null, 2) + '\n');
  fs.writeFileSync(path.join(stage, 'README.txt'), component === 'client'
    ? `WebMux ${metadata.version} — native client

Copy bin/${goos === 'windows' ? 'webmux-client.exe' : 'webmux-client'} to a directory on PATH.
Run it explicitly with --server https://WEBMUX_HOST:8443 --background, then link
the printed pairing code in the WebMux SSH tile. Installation does not start or
enroll the client. No WebMux server or browser assets are included.
Documentation: https://github.com/jordanhubbard/webmux/blob/main/docs/managed-hosts.md
`
    : `WebMux ${metadata.version} — native server${component === 'all' ? ' and client' : ''}

Run bin/${goos === 'windows' ? 'webmux.exe' : 'webmux'} and open http://localhost:8080.
The executable embeds the browser UI and configuration defaults.
No Node.js runtime or adjacent asset directories are required; you may copy the
executable anywhere. Optional Windows service support stays in this archive.
Configuration/state default to ~/.config/webmux; WEBMUX_HOME overrides this.
WEBMUX_ROOT or --root optionally supplies external web/ and config.defaults/
directories. Missing directories use embedded assets.
Never run multiple servers against the same writable home.
Reinstall or reconfigure services that still point to an older Node installation.
${goos === 'darwin' ? 'macOS: keep WebMux.app at a stable location such as ~/Applications. Run its\nContents/MacOS/webmux executable for the app identity used by privacy settings.\nComplete Mac permissions in the browser after starting the server.\nThe flat bin/webmux executable is retained for CLI compatibility.\n' : ''}
${goos === 'windows' ? 'Service installation: from an elevated terminal, run bin\\webmux-service.cmd install\nwith -WebMuxHome pointing to isolated state. The existing account prompt and\noptional -LocalSystem switch apply; no Node.js runtime is needed.\n' : ''}
OpenSSH is required for SSH; mosh, sshpass and tmux depend on selected features.
RDP requires an operator-managed guacd. These external tools are not bundled.
${component === 'all' ? `The standalone bin/${goos === 'windows' ? 'webmux-client.exe' : 'webmux-client'} connects a user-managed host outward to a WebMux server.\n` : ''}
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
