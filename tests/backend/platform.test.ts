import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import {
  commandShell,
  homeDirectory,
  interactiveShell,
  resolveExecutable,
  tempDirectory,
} from '@backend/services/platform';

describe('platform helpers', () => {
  it('uses Node home and temporary directories', () => {
    expect(homeDirectory()).toBe(os.homedir());
    expect(tempDirectory()).toBe(os.tmpdir());
  });

  it('resolves executables from PATH without invoking a shell', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-platform-'));
    const command = process.platform === 'win32' ? 'webmux-test-command.exe' : 'webmux-test-command';
    const executable = path.join(dir, command);
    fs.writeFileSync(executable, '#!/bin/sh\nexit 0\n');
    fs.chmodSync(executable, 0o755);
    try {
      expect(resolveExecutable(command, { PATH: dir }, process.platform)).toBe(executable);
      expect(resolveExecutable('missing-command', { PATH: dir }, process.platform)).toBeNull();
    } finally {
      fs.rmSync(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    }
  });

  it('selects cmd.exe for Windows command and interactive shells', () => {
    const env = { COMSPEC: 'C:\\Windows\\System32\\cmd.exe' };
    expect(commandShell('win32', env)).toEqual({
      command: env.COMSPEC,
      args: ['/d', '/s', '/c'],
    });
    expect(interactiveShell('win32', env)).toEqual({ command: env.COMSPEC, args: [] });
  });

  it('selects the configured POSIX shell', () => {
    const env = { SHELL: '/bin/zsh' };
    expect(commandShell('darwin', env)).toEqual({ command: '/bin/zsh', args: ['-c'] });
    expect(interactiveShell('darwin', env)).toEqual({ command: '/bin/zsh', args: ['-l'] });
  });
});
