import fs from 'node:fs';
import type { APIRequestContext, Page, TestInfo } from '@playwright/test';

import { federationFrameFacts, federationRouteFacts } from './federation-diagnostic-facts.mts';

const states = new Set(['connecting', 'connected', 'disconnected']);
const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);

export function createFederationDiagnostics(page: Page, request: APIRequestContext, info: TestInfo, peerURL: string, startsFile: string) {
  const enabled = process.env.WEBMUX_FEDERATION_DIAGNOSTICS === '1';
  const started = Date.now();
  let phase = 'startup', sequence = 0, sockets = 0, recording = true;
  type Event = { sequence: number; elapsedMs: number; phase: string; [key: string]: unknown };
  const firstEvents: Event[] = [], lastEvents: Event[] = [];
  const append = (value: Record<string, unknown>) => {
    if (!enabled || !recording) return;
    const event = { ...value, sequence: ++sequence, elapsedMs: Date.now() - started, phase };
    if (firstEvents.length < 64) firstEvents.push(event);
    lastEvents.push(event);
    if (lastEvents.length > 256) lastEvents.shift();
  };
  if (enabled) page.on('websocket', socket => {
    const role = federationRouteFacts(socket.url());
    if (!role || sockets >= 64) return;
    const id = ++sockets;
    append({ socket: id, role, kind: 'created' });
    socket.on('framereceived', event => append({ socket: id, role, kind: 'received', ...federationFrameFacts(event.payload) }));
    socket.on('framesent', event => append({ socket: id, role, kind: 'sent', ...federationFrameFacts(event.payload) }));
    socket.on('close', () => append({ socket: id, role, kind: 'closed' }));
    socket.on('socketerror', () => append({ socket: id, role, kind: 'error' }));
  });
  const bounded = async <T,>(action: Promise<T>): Promise<T | { unavailable: true }> => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    try { return await Promise.race([action, new Promise<{ unavailable: true }>(resolve => { timer = setTimeout(() => resolve({ unavailable: true }), 1200); })]); }
    catch { return { unavailable: true }; }
    finally { clearTimeout(timer); }
  };
  const sessions = async (base: string) => {
    try {
      const response = await request.get(base + '/api/sessions', { timeout: 1000 });
      if (response.status() !== 200) return { status: response.status() };
      const values: unknown = await response.json();
      return { status: response.status(), sessions: Array.isArray(values) ? values.slice(0, 8).filter(object).map(value => ({
        state: states.has(String(value.state)) ? String(value.state) : 'other',
        cols: Number.isSafeInteger(value.cols) ? value.cols : null, rows: Number.isSafeInteger(value.rows) ? value.rows : null,
        // A current session count is not evidence of a unique provider process.
      })) : [] };
    } catch { return { unavailable: true }; }
  };
  return {
    enabled,
    phase(value: string) { phase = value; append({ kind: 'phase' }); },
    async capture(completed: boolean, peer: { pid?: number; exitCode: number | null; signalCode: string | null }, diagnosticsBytes: number) {
      if (!enabled) return;
      append({ kind: 'capture', completed });
      recording = false;
      // Freeze events before diagnostics and before fixture cleanup destroys the
      // failed view/provider. All extra requests here are read-only and bounded.
      const [views, controller, remote] = await Promise.all([
        bounded(page.locator('[data-testid^="server-terminal-"]').evaluateAll(elements => elements.slice(0, 8).map(element => {
          const text = element.querySelector('.xterm-rows')?.textContent ?? '';
          const viewer = element.querySelector('[title="Browser viewer connection"]')?.textContent ?? '';
          const provider = element.querySelector('[title="Provider state reported by the owning server"]')?.textContent ?? '';
          return { role: element.getAttribute('data-server-id') === 'local' ? 'local' : 'peer',
            viewer: ['Viewer: connecting', 'Viewer: connected', 'Viewer: disconnected'].includes(viewer) ? viewer : 'other',
            provider: ['connecting', 'connected', 'disconnected'].includes(provider) ? provider : 'other',
            blank: !text.trim(), textLength: text.length,
            readyPIDs: [...text.matchAll(/FEDERATION_READY:([1-9]\d{0,9})/g)].slice(0, 8).map(value => Number(value[1])),
            nativeInputProof: text.includes('FEDERATION_REPLY:native-input-proof') };
        }))), sessions(''), sessions(peerURL),
      ]);
      const starts: { pid: number; instance: string; role: string; presence: string }[] = [];
      if (fs.existsSync(startsFile) && fs.statSync(startsFile).size <= 65536) {
        for (const line of fs.readFileSync(startsFile, 'utf8').split('\n').filter(Boolean).slice(0, 32)) {
          let value: unknown;
          try { value = JSON.parse(line); } catch { continue; }
          if (!object(value) || !Number.isSafeInteger(value.pid) || Number(value.pid) < 1 || Number(value.pid) >= 2 ** 31
            || typeof value.instance !== 'string' || !/^[a-f0-9-]{36}$/.test(value.instance) || !['local', 'peer'].includes(String(value.role))) continue;
          let presence = 'exists';
          try { process.kill(Number(value.pid), 0); }
          catch (error) { presence = (error as NodeJS.ErrnoException).code === 'ESRCH' ? 'missing' : 'unknown'; }
          starts.push({ pid: Number(value.pid), instance: value.instance, role: String(value.role), presence });
        }
      }
      const receipt = { completed, phase, platform: process.platform, architecture: process.arch, node: process.version,
        peer: { pid: peer.pid, exited: peer.exitCode !== null || peer.signalCode !== null }, diagnosticsBytes,
        sockets, events: sequence, firstEvents, lastEvents, views, controller, remote, starts,
        limits: { firstEvents: 64, lastEvents: 256, sockets: 64, starts: 32 },
        scope: 'Optional test-only observations before cleanup. PID presence alone is not process identity. No raw frame bodies, URL/query strings, tokens, error text or native account data retained. Observation can affect timing; a green sample does not establish a repair.' };
      const file = info.outputPath('federation-diagnostics.json');
      fs.writeFileSync(file, JSON.stringify(receipt));
      await info.attach('bounded federation lifecycle', { path: file, contentType: 'application/json' });
    },
  };
}
