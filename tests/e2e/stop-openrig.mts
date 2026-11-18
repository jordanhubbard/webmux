// The daemon belongs to Playwright's shared web server, not a test worker.
// afterAll also runs when a failed test causes its worker to be discarded.
import { existsSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

export default function stopOpenRig(): void {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const executable = path.resolve(import.meta.dirname, '.test-home/openrig-bin/rig');
  if (!existsSync(executable)) return;
  // Global teardown runs once, before the web-server process group disappears.
  // Keep the server's own exit cleanup as an idempotent fallback.
  const stopped = spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, encoding: 'utf8' });
  if (stopped.status !== 0) throw new Error(`Cannot stop isolated Rig fixture: ${stopped.stdout} ${stopped.stderr}`);
}
