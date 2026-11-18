// Supported Rig launch_wrapper entry point; not wired into WebMux creation yet.
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { filterRigCodexLaunch } from './codex-launch-policy.mts';
import { nativeRigToolDirectory, restoreCodexLaunchEnvironment } from './codex-launch-environment.mts';
import { receiveCodexEnvironment } from './codex-environment-channel.mts';

try {
  if (!['darwin', 'linux'].includes(process.platform) || typeof process.execve !== 'function') throw new Error();
  const [manifest, target, ...args] = process.argv.slice(2);
  if (!manifest || !target || !path.isAbsolute(manifest) || !path.isAbsolute(target)
    || !process.env.OPENRIG_NODE_ID || !process.env.OPENRIG_OCCUPANT_GENERATION) throw new Error();
  const directory = fs.statSync(path.dirname(manifest));
  if (directory.uid !== process.getuid!() || (directory.mode & 0o777) !== 0o700 || fs.realpathSync(manifest) !== manifest) throw new Error();
  const fd = fs.openSync(manifest, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  let value: unknown;
  try {
    const stat = fs.fstatSync(fd);
    if (!stat.isFile() || stat.uid !== process.getuid!() || stat.nlink !== 1 || (stat.mode & 0o777) !== 0o600 || stat.size > 262144) throw new Error();
    const data = Buffer.alloc(262145), count = fs.readSync(fd, data, 0, data.length, 0);
    const after = fs.fstatSync(fd);
    if (count !== stat.size || after.size !== stat.size || after.mtimeMs !== stat.mtimeMs || after.ctimeMs !== stat.ctimeMs) throw new Error();
    value = JSON.parse(data.subarray(0, count).toString('utf8'));
  } finally { fs.closeSync(fd); }
  let environment = process.env;
  if (value && typeof value === 'object' && 'contract' in value && value.contract === 'webmux-codex-launch/v2') {
    const envelope = value as { contract: string; plan: { home: string; cwd: string }; environment: import('./codex-launch-environment.mts').CodexLaunchEnvironment; environmentSocket: string };
    if (Object.keys(envelope).some(key => !['contract', 'plan', 'environment', 'environmentSocket'].includes(key)) || !envelope.plan
      || typeof envelope.environmentSocket !== 'string') throw new Error();
    const supplied = await receiveCodexEnvironment(envelope.environmentSocket, envelope.environment);
    environment = restoreCodexLaunchEnvironment(envelope.environment, supplied, envelope.plan.home, envelope.plan.cwd, process.env);
    environment.PATH = nativeRigToolDirectory(process.env) + path.delimiter + environment.PATH;
    value = envelope.plan;
  }
  const launch = filterRigCodexLaunch(value, args);
  if (fs.realpathSync(target) !== launch.executable || fs.realpathSync(process.cwd()) !== (value as { cwd: string }).cwd
    || fs.realpathSync(environment.CODEX_HOME || '') !== (value as { home: string }).home) throw new Error();
  const stat = fs.statSync(launch.executable);
  if (!stat.isFile() || stat.size > 512 * 1024 * 1024) throw new Error();
  const digest = createHash('sha256');
  for await (const bytes of fs.createReadStream(launch.executable, { signal: AbortSignal.timeout(5000) })) digest.update(bytes);
  if (digest.digest('hex') !== (value as { executableSHA256: string }).executableSHA256) throw new Error();
  process.execve(launch.executable, [launch.executable, ...launch.args], environment);
} catch {
  // No argv, account identity, private paths, manifest contents or native stderr.
  process.stderr.write('WebMux: native Codex launch policy could not be preserved.\n');
  process.exitCode = 1;
}
