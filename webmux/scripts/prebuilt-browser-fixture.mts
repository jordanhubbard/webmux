import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';

// A disposable hosted task cannot share the runner's compiler environment.
// Admit only the exact image built before that task; ordinary tests still build.
export function prebuiltBrowserFixture(env: NodeJS.ProcessEnv, platform: string): string | undefined {
  const image = env.WEBMUX_E2E_PREBUILT_BINARY;
  const digest = env.WEBMUX_E2E_PREBUILT_SHA256;
  if (!image && !digest) return undefined;
  if (platform !== 'win32' || env.GITHUB_ACTIONS !== 'true' || env.RUNNER_ENVIRONMENT !== 'github-hosted') throw Error('Prebuilt browser fixtures require a disposable Windows Actions runner');
  if (!image || !digest || !/^[a-f0-9]{64}$/.test(digest) || !path.isAbsolute(image)) throw Error('Incomplete prebuilt browser fixture identity');
  const stat = fs.lstatSync(image);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 128 * 1024 * 1024) throw Error('Invalid prebuilt browser fixture image');
  if (createHash('sha256').update(fs.readFileSync(image)).digest('hex') !== digest) throw Error('Prebuilt browser fixture image changed');
  return image;
}
