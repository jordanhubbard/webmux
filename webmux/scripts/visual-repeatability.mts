import { spawnSync } from 'node:child_process';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
// Check visual repeatability on the current Go build, in both auth modes.
// This is not a cross-version regression baseline: the first run creates it.
for (const auth of ['none', 'local']) {
  for (const update of ['all', 'none'] as const) {
    const result = spawnSync(process.execPath, [path.join(root, 'scripts', 'run-playwright.mts'), 'visual.spec.ts', `--update-snapshots=${update}`], {
      cwd: root, stdio: 'inherit', env: { ...process.env, WEBMUX_VISUAL_PARITY: '1', WEBMUX_E2E_AUTH: auth },
    });
    if (result.error) throw result.error;
    if (result.status !== 0) process.exit(result.status ?? 1);
  }
}
