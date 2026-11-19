import type { Page } from '@playwright/test';

type ReplayBarrier = { armed: boolean; held: number; delivered: number; callbackError: boolean; arm(): void; release(): void };
declare global { interface Window { federationReplayBarrier?: ReplayBarrier } }

// Negative-control scheduling barrier only. Keep the native WebSocket and its
// real frames, but postpone the application's output callback until the test
// has observed the existing viewer outside the viewport. Never synthesize data.
export async function installFederationReplayBarrier(page: Page) {
  await page.addInitScript(() => {
    const descriptor = Object.getOwnPropertyDescriptor(WebSocket.prototype, 'onmessage')!;
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    const state: ReplayBarrier = { armed: false, held: 0, delivered: 0, callbackError: false,
      arm() { state.armed = true; }, release() { release(); } };
    window.federationReplayBarrier = state;
    Object.defineProperty(WebSocket.prototype, 'onmessage', { ...descriptor,
      set(this: WebSocket, listener: ((event: MessageEvent) => void) | null) {
        if (!listener) { descriptor.set!.call(this, listener); return; }
        let queue = Promise.resolve();
        descriptor.set!.call(this, (event: MessageEvent) => {
          let output = false;
          try { output = JSON.parse(event.data).type === 'output'; } catch { /* Forward unchanged. */ }
          const hold = state.armed && /\/api\/servers\/(?!local\/)[^/]+\/term\//.test(new URL(this.url).pathname) && output;
          if (hold) state.held++;
          queue = queue.then(async () => {
            if (hold) await gate;
            listener.call(this, event);
            if (hold) state.delivered++;
          }).catch(() => { state.callbackError = true; });
        });
      },
    });
  });
}

export async function releaseFederationReplayOffscreen(page: Page) {
  const facts = await page.locator('[data-testid^="server-terminal-"]:not([data-server-id="local"]) .xterm-screen').evaluate(screen => new Promise<{ offscreen: boolean }>(resolve => {
    const workspace = screen.closest('.server-workspace')!;
    workspace.scrollTop = 0;
    const observer = new IntersectionObserver(entries => {
      observer.disconnect();
      requestAnimationFrame(() => requestAnimationFrame(() => resolve({ offscreen: !entries[0].isIntersecting })));
    });
    observer.observe(screen);
  }));
  if (!facts.offscreen) throw new Error('Replay control requires an actually offscreen viewer');
  await page.evaluate(() => window.federationReplayBarrier!.release());
}
