import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';

type Reply = { status(): number; json(): Promise<unknown> };
type Options = { data: object; timeout?: number };
type FixtureRequest = {
  get(url: string): Promise<Reply>;
  post(url: string, options: Options): Promise<Reply>;
  patch(url: string, options: Options): Promise<Reply>;
  delete(url: string, options?: Options): Promise<Reply>;
};
type Attachment = { attach(name: string, data: { contentType: string; body: string }): Promise<void> };
type ObservedResponse = Reply & { url(): string; ok(): boolean; request(): { method(): string; postDataJSON(): unknown } };
type FixturePage = { on(event: 'response', listener: (response: ObservedResponse) => void): unknown };
type Lifecycle = { contract?: string; state?: string; selection?: string; operation_id?: string };
type FixtureSession = { id?: string; local_lifecycle?: Lifecycle };
type BrowserFrame = { control?: { owned?: boolean; available?: boolean; token?: string } };

// A browser fixture owns its session, not every terminal in the workspace.
// Keep the exact lease issued to this viewer; cleanup must not steal another one.
export function observeFixtureBrowser(page: FixturePage, request: FixtureRequest, id: string) {
  let lease: { viewer: string; controlToken: string } | undefined;
  const pending = new Set<Promise<void>>();
  const endpoint = `/api/sessions/${id}/browser`;
  page.on('response', response => {
    if (new URL(response.url()).pathname !== endpoint || response.request().method() !== 'POST' || !response.ok()) return;
    const task = (async () => {
      const body = response.request().postDataJSON() as { viewer?: unknown } | null;
      const frame = await response.json() as BrowserFrame;
      if (frame.control?.owned && frame.control.token && typeof body?.viewer === 'string') {
        lease = { viewer: body.viewer, controlToken: frame.control.token };
      }
    })().catch(() => { /* A lost frame supplies no new cleanup authority. */ });
    pending.add(task); void task.finally(() => pending.delete(task));
  });
  return async () => {
    await Promise.all([...pending]);
    if (!lease) return;
    let response = await request.delete(endpoint, { data: lease });
    if (response.status() === 409) {
      // Return to terminal releases its viewer lease but retains the browser.
      // Claim only an actually available lease, using the same fixture viewer.
      // A concurrent owner's claim remains authoritative; never force takeover.
      const observed = await request.post(endpoint, { data: { action: 'frame', ...lease } });
      assert.equal(observed.status(), 200, 'fixture browser availability is unconfirmed');
      const frame = await observed.json() as BrowserFrame;
      assert.equal(frame.control?.available, true, 'another viewer owns the fixture browser; retain its session');
      const claimed = await request.post(endpoint, { data: { action: 'claim', ...lease } });
      assert.equal(claimed.status(), 200, 'fixture browser lease could not be reclaimed');
      const result = await claimed.json() as BrowserFrame;
      assert(result.control?.owned && result.control.token, 'another viewer claimed the browser; cleanup did not take over');
      lease = { viewer: lease.viewer, controlToken: result.control.token };
      response = await request.delete(endpoint, { data: lease });
    }
    assert.equal(response.status(), 204, 'fixture browser lease could not be ended; retain its session');
  };
}

// DELETE is intentionally insufficient for durable local Windows jobs. Never
// discard their access record after an unavailable owner or an uncertain Stop.
export async function disposeFixtureTerminal(request: FixtureRequest, id: string, info: Attachment) {
  const endpoint = `/api/sessions/${id}`;
  const response = await request.get(endpoint);
  if (response.status() === 404) return;
  assert.equal(response.status(), 200, 'fixture session could not be reviewed');
  const session = await response.json() as FixtureSession;
  assert.equal(session.id, id, 'fixture lookup changed the selected session');
  const lifecycle = session.local_lifecycle;
  const retain = async (reason: string, operation?: string) => {
    const hidden = await request.patch(endpoint, { data: { minimized: true } });
    assert.equal(hidden.status(), 200, 'unresolved fixture session could not be retained in the window list');
    await info.attach('retained-terminal-lifecycle', {
      contentType: 'application/json',
      body: JSON.stringify({ id, reason, operation, state: lifecycle?.state, savedAccessRetained: true }),
    });
  };
  if (lifecycle?.contract === 'webmux-local-provider-lifecycle/v1' && lifecycle.state !== 'stopped') {
    if (!lifecycle.selection || lifecycle.state === 'unavailable') {
      await retain('Original provider is unavailable; cleanup did not dispatch Stop or discard its recovery record.');
      return;
    }
    const operation = {
      id: lifecycle.operation_id || randomUUID(), selection: lifecycle.selection,
      mode: lifecycle.operation_id ? 'inspect' : 'stop', confirm: !lifecycle.operation_id,
    };
    let stopped;
    try { stopped = await request.post(endpoint + '/local-stop', { data: operation, timeout: 15_000 }); }
    catch {
      await retain('Stop reply was lost; do not dispatch another request.', operation.id);
      return;
    }
    assert([200, 202].includes(stopped.status()), `fixture Stop refused (${stopped.status()}); keep the original session`);
    const result = await stopped.json() as { outcome?: string; session?: FixtureSession };
    assert.equal(result.session?.id, id, 'Stop returned another session');
    assert.equal(result.session?.local_lifecycle?.operation_id, operation.id, 'Stop returned another operation');
    assert.equal(result.session?.local_lifecycle?.selection, operation.selection, 'Stop returned another provider selection');
    if (result.outcome !== 'stopped') {
      assert.equal(result.outcome, 'uncertain', 'fixture Stop returned an unknown outcome');
      await retain('Original Stop is unconfirmed; inspection/continuation remains explicit.', operation.id);
      return;
    }
    assert.equal(stopped.status(), 200, 'a confirmed fixture Stop must return success');
  }
  const deleted = await request.delete(endpoint);
  assert.equal(deleted.status(), 204, 'fixture deletion must follow the original confirmed Stop');
}
