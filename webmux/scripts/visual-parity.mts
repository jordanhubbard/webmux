import { spawnSync } from 'node:child_process';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
// Both runs use the same build, browser, platform, viewport and fonts. Only the
// Node run can create baselines; Go must match them with zero changed pixels.
for (const backend of ['node', 'go']) {
  const result = spawnSync(process.execPath, [path.join(root, 'scripts', 'run-playwright.mts'), `--backend=${backend}`, 'visual.spec.ts', `--update-snapshots=${backend === 'node' ? 'all' : 'none'}`], {
    cwd: root, stdio: 'inherit', env: { ...process.env, WEBMUX_VISUAL_PARITY: '1' },
  });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
