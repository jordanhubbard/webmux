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
    expect(commandShell('echo hello', 'win32', env)).toEqual({
      command: env.COMSPEC,
      args: '/d /s /c "echo hello"',
    });
    expect(interactiveShell('win32', env)).toEqual({ command: env.COMSPEC, args: [] });
  });

  it('selects the configured POSIX shell', () => {
    const env = { SHELL: '/bin/zsh' };
    expect(commandShell('echo "hello world"', 'darwin', env)).toEqual({ command: '/bin/zsh', args: ['-c', 'echo "hello world"'] });
    expect(interactiveShell('darwin', env)).toEqual({ command: '/bin/zsh', args: ['-l'] });
  });

  it('preserves quoted Windows executable paths and arguments without argv escaping', () => {
    const commandLine = '"C:\\Program Files\\nodejs\\node.exe" -e "console.log(\'hello world\')"';
    const shell = commandShell(commandLine, 'win32', { COMSPEC: 'cmd.exe' });
    expect(shell.args).toBe(`/d /s /c "${commandLine}"`);
  });
});
