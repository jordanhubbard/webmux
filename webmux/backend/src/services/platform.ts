import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

export interface ShellCommand {
  command: string;
  args: string[];
}

function isExecutable(file: string, platform: NodeJS.Platform): boolean {
  try {
    if (!fs.statSync(file).isFile()) return false;
    if (platform !== 'win32') fs.accessSync(file, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

/** Resolve an executable exactly as the target platform's command search would. */
export function resolveExecutable(
  name: string,
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
): string | null {
  if (!name || name.includes('\0')) return null;

  const pathImpl = platform === 'win32' ? path.win32 : path.posix;
  const hasPath = pathImpl.isAbsolute(name) || name.includes('/') || name.includes('\\');
  const directories = hasPath
    ? ['']
    : (env.PATH || env.Path || '')
      .split(platform === 'win32' ? ';' : ':')
      .map(entry => entry.replace(/^"|"$/g, ''));
  const extensions = platform === 'win32' && !pathImpl.extname(name)
    ? (env.PATHEXT || env.PathExt || '.COM;.EXE;.BAT;.CMD').split(';').filter(Boolean)
    : [''];

  for (const directory of directories) {
    const base = hasPath ? name : pathImpl.join(directory || '.', name);
    for (const extension of extensions) {
      const candidate = `${base}${extension}`;
      if (isExecutable(candidate, platform)) return pathImpl.resolve(candidate);
    }
  }
  return null;
}

export function homeDirectory(): string {
  return os.homedir();
}

export function tempDirectory(): string {
  return os.tmpdir();
}

export function commandShell(
  platform: NodeJS.Platform = process.platform,
  env: NodeJS.ProcessEnv = process.env,
  resolver: typeof resolveExecutable = resolveExecutable,
): ShellCommand {
  if (platform === 'win32') {
    const command = env.COMSPEC || resolver('cmd.exe', env, platform);
    if (!command) throw new Error('Windows command shell (cmd.exe) was not found');
    return { command, args: ['/d', '/s', '/c'] };
  }
  return { command: env.SHELL?.trim() || '/bin/sh', args: ['-c'] };
}

export function interactiveShell(
  platform: NodeJS.Platform = process.platform,
  env: NodeJS.ProcessEnv = process.env,
  resolver: typeof resolveExecutable = resolveExecutable,
): ShellCommand {
  if (platform === 'win32') {
    const command = env.COMSPEC || resolver('cmd.exe', env, platform);
    if (!command) throw new Error('Windows command shell (cmd.exe) was not found');
    return { command, args: [] };
  }
  return { command: env.SHELL?.trim() || '/bin/sh', args: ['-l'] };
}
