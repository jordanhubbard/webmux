import { spawnSync } from 'node:child_process';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
// All runs use the same build, browser, platform, viewport and fonts. Only the
// first Node run can create baselines; Go must match with zero changed pixels.
// A second Node run verifies that the baseline itself is repeatable before
// attributing any difference to Go. Neither comparison may update the baseline.
for (const auth of ['none', 'local']) {
  for (const [backend, update] of [['node', 'all'], ['node', 'none'], ['go', 'none']] as const) {
    const result = spawnSync(process.execPath, [path.join(root, 'scripts', 'run-playwright.mts'), `--backend=${backend}`, 'visual.spec.ts', `--update-snapshots=${update}`], {
      cwd: root, stdio: 'inherit', env: { ...process.env, WEBMUX_VISUAL_PARITY: '1', WEBMUX_E2E_AUTH: auth },
    });
    if (result.error) throw result.error;
    if (result.status !== 0) process.exit(result.status ?? 1);
  }
}
