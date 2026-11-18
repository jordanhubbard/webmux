// Exercise a real Playwright worker failure against the shared native Rig fixture.
// The child intentionally exits 1; acceptance requires exactly that failure and
// a successful original dashboard workflow in a different worker, then cleanup.
import { readFileSync, writeFileSync, mkdirSync, existsSync, unlinkSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';

type TestResult = { status: string; errors?: { message?: string }[] };
type TestCase = { status: string; results: TestResult[] };
type Suite = { specs?: { tests: TestCase[] }[]; suites?: Suite[] };
type Report = { suites: Suite[]; stats: { unexpected: number }; errors?: unknown[] };

const root = path.resolve(import.meta.dirname, '..');
const directory = path.join(root, 'tests/e2e');
const outputRoot = path.resolve(process.argv[2] ?? path.join(root, 'webmux/test-results/rig-worker-lifecycle'));
const output = path.join(outputRoot, 'run-' + randomUUID());
mkdirSync(output, { recursive: true });
const report = path.join(output, 'report.json');
const before = path.join(output, 'original-daemon.json');
const after = path.join(output, 'replacement-worker.json');
const file = path.join(directory, `openrig-lifecycle-${randomUUID()}.spec.ts`);
let source = readFileSync(path.join(directory, 'openrig.spec.ts'), 'utf8');
const marker = 'INTENTIONAL_RIG_WORKER_REPLACEMENT';
const failurePoint = '  test.setTimeout(120_000);';
const dashboardPoint = '  let id: string | null = null;';
if (!source.includes(failurePoint) || !source.includes(dashboardPoint)) throw new Error('Rig lifecycle control source anchors changed');
source = source.replace(failurePoint, failurePoint + `
  writeFileSync(${JSON.stringify(before)}, JSON.stringify({worker: info.workerIndex, daemon: JSON.parse(readFileSync(path.resolve(__dirname, '.test-home/openrig/daemon.json'), 'utf8'))}));
  throw new Error(${JSON.stringify(marker)});`);
source = source.replace(dashboardPoint, dashboardPoint + `
  const original = JSON.parse(readFileSync(${JSON.stringify(before)}, 'utf8'));
  const current = JSON.parse(readFileSync(path.join(home, 'openrig/daemon.json'), 'utf8'));
  const healthy = await fetch(daemon + '/healthz').then(r => r.ok).catch(() => false);
  writeFileSync(${JSON.stringify(after)}, JSON.stringify({worker: info.workerIndex, originalPID: original.daemon.pid, currentPID: current.pid, healthy}));
  expect(info.workerIndex).not.toBe(original.worker);
  expect(current.pid).toBe(original.daemon.pid);
  expect(current.port).toBe(original.daemon.port);
  expect(new URL(daemon).port).toBe(String(original.daemon.port));
  expect(healthy, 'Original Rig daemon must survive the discarded test worker').toBe(true);`);
writeFileSync(file, source, { flag: 'wx' });
try {
  const run = spawnSync(process.execPath, [path.join(root, 'webmux/scripts/run-playwright.mts'), path.basename(file),
    '--grep', 'managed agents|real diagnostic', '--reporter=json', '--output=' + path.join(output, 'browser')], {
    cwd: path.join(root, 'webmux'), env: { ...process.env, WEBMUX_REAL_OPENRIG: '1', PLAYWRIGHT_JSON_OUTPUT_FILE: report },
    encoding: 'utf8', timeout: 180000, maxBuffer: 8 << 20,
  });
  writeFileSync(path.join(output, 'child.log'), run.stdout + run.stderr);
  if (run.error) throw run.error;
  const result = JSON.parse(readFileSync(report, 'utf8')) as Report;
  const tests: TestCase[] = [];
  const visit = (suite: Suite) => { for (const spec of suite.specs ?? []) tests.push(...spec.tests); for (const child of suite.suites ?? []) visit(child); };
  for (const suite of result.suites) visit(suite);
  const first = tests.find(t => t.results.some((r: TestResult) => r.errors?.some((e: { message?: string }) => e.message?.includes(marker))));
  const second = tests.find(t => t !== first);
  if (run.status !== 1 || tests.length !== 2 || result.stats.unexpected !== 1 || result.errors?.length || !first || first.results.length !== 1 || first.results[0].status !== 'failed' || first.results[0].errors?.length !== 1 || second?.status !== 'expected' || second.results.length !== 1 || second.results[0].status !== 'passed') {
    throw new Error('Expected exactly one deliberate failure and a passing dashboard in a replacement worker');
  }
  const original = JSON.parse(readFileSync(before, 'utf8'));
  const replacement = JSON.parse(readFileSync(after, 'utf8'));
  if (replacement.worker === original.worker || replacement.originalPID !== replacement.currentPID || !replacement.healthy) throw new Error('Shared daemon identity changed');
  let alive = true;
  try { process.kill(original.daemon.pid, 0); } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ESRCH') alive = false; else throw error; }
  if (alive || existsSync(path.join(directory, '.test-home/openrig/daemon.json'))) throw new Error('Shared Rig daemon was not cleaned up after the complete run');
  writeFileSync(path.join(output, 'summary.json'), JSON.stringify({ intentionalFailures: 1, dashboardPassed: true, replacementWorker: true, originalDaemonPreserved: true, daemonStoppedAfterRun: true }, null, 2) + '\n');
  console.log('Rig shared fixture survives worker replacement; final daemon cleanup verified');
} finally {
  unlinkSync(file);
}
