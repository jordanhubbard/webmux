// Opt-in helper for the native Go shared-reader qualification. Receives only
// the disposable WebMux browser token over stdin; MAC credentials stay in Go.
// No route interception or response fixtures: the production UI reads real MAC
// through the running isolated WebMux server. Artifacts remain private/local.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { chromium, expect, type Page } from '@playwright/test';

interface Input {
  origin: string; token: string; session_id: string; project_key: string;
  task_ids: string[]; artifact_directory: string; executable: string;
}
const chunks: Buffer[] = [];
let bytes = 0;
for await (const chunk of process.stdin) {
  const data = Buffer.from(chunk as Uint8Array); bytes += data.length;
  if (bytes > 1 << 20) throw new Error('Native browser input exceeds its bound');
  chunks.push(data);
}
const input = JSON.parse(Buffer.concat(chunks).toString('utf8')) as Input;
const origin = new URL(input.origin);
assert.equal(origin.protocol, 'http:'); assert.equal(origin.hostname, '127.0.0.1');
assert.ok(origin.port && !origin.username && !origin.password && !origin.search && !origin.hash);
assert.ok(input.token && input.session_id && input.project_key && input.task_ids.length > 0);
assert.ok(path.isAbsolute(input.artifact_directory) && path.isAbsolute(input.executable));
fs.mkdirSync(input.artifact_directory, { recursive: true, mode: 0o700 });
const artifacts = fs.mkdtempSync(path.join(input.artifact_directory, 'native-browser-'));
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
let page: Page | undefined;
try {
  browser = await chromium.launch({ executablePath: input.executable, headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1100 } });
  await context.addInitScript(({ token }) => localStorage.setItem('webmux_token', token), { token: input.token });
  page = await context.newPage();
  page.on('pageerror', error => fs.appendFileSync(path.join(artifacts, 'browser-errors-private.txt'), `${error.stack ?? error.message}\n`, { mode: 0o600 }));
  await page.goto(input.origin);
  const tile = page.getByTestId(`tile-cell-${input.session_id}`);
  await expect(tile.getByRole('button', { name: 'Project activity', exact: true })).toBeVisible();
  const response = page.waitForResponse(value => {
    const url = new URL(value.url());
    return url.origin === origin.origin && url.pathname === `/api/session-coordination/projects/${input.session_id}`;
  });
  await tile.getByRole('button', { name: 'Project activity', exact: true }).click();
  const upstream = await response;
  assert.equal(upstream.status(), 200);
  const view = await upstream.json() as {
    schema: string; subject: { session_id: string }; source_state: string; source_task_link_state: string;
    source: { commit: string };
    project: { project_key: string; conflict_verdict: string; ordering_verdict: string; tasks: { task: { task_id: string } }[] };
  };
  assert.equal(view.schema, 'webmux.mac.session-project-view.v1');
  assert.equal(view.subject.session_id, input.session_id);
  assert.equal(view.project.project_key, input.project_key);
  assert.equal(view.project.conflict_verdict, 'unknown'); assert.equal(view.project.ordering_verdict, 'unknown');
  assert.equal(view.source_state, 'observed'); assert.equal(view.source_task_link_state, 'unlinked');
  const shared = view.project.tasks.filter(row => input.task_ids.includes(row.task.task_id));
  assert.ok(shared.length > 0, 'Fresh native browser and MCP pages have no shared task identity');
  const dialog = page.getByRole('dialog', { name: 'Project activity', exact: true });
  await expect(dialog.getByText(input.project_key, { exact: true })).toBeVisible();
  await expect(dialog.getByText(shared[0].task.task_id, { exact: true })).toBeVisible();
  await expect(dialog.getByText(/Assessment: unknown. Task scope/)).toBeVisible();
  await expect(dialog.getByText(/Assessment: unknown. The source roadmap/)).toBeVisible();
  await page.screenshot({ path: path.join(artifacts, 'project-native.png'), fullPage: true });
  await dialog.getByRole('heading', { name: 'Source context', exact: true }).scrollIntoViewIfNeeded();
  await expect(dialog.getByText(view.source.commit, { exact: true })).toBeVisible();
  await expect(dialog.getByText(/not linked to a MAC task or roadmap revision/)).toBeVisible();
  await page.screenshot({ path: path.join(artifacts, 'project-native-source.png'), fullPage: true });
  fs.writeFileSync(path.join(artifacts, 'project-private.json'), JSON.stringify(view, null, 2), { mode: 0o600 });
  for (const name of ['project-native.png', 'project-native-source.png']) fs.chmodSync(path.join(artifacts, name), 0o600);
  process.stdout.write(JSON.stringify({ completed: true, shared_task_identities: shared.length, source_observed: true, route_interception: false, artifact_directory: artifacts }));
} catch (error) {
  // Playwright failures can quote private task text. Retain them locally and
  // emit only a generic failure to the parent/test console.
  fs.writeFileSync(path.join(artifacts, 'failure-private.txt'), error instanceof Error ? error.stack ?? error.message : String(error), { mode: 0o600 });
  if (page) {
    try {
      fs.writeFileSync(path.join(artifacts, 'failure-private.html'), await page.content(), { mode: 0o600 });
      await page.screenshot({ path: path.join(artifacts, 'failure-private.png'), fullPage: true });
      fs.chmodSync(path.join(artifacts, 'failure-private.png'), 0o600);
    } catch { /* Preserve the original error if Chrome has already closed. */ }
  }
  process.stderr.write('Native browser qualification failed; details retained in its private artifact directory.\n');
  process.exitCode = 1;
} finally {
  await browser?.close();
}
