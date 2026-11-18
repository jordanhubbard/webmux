// Notifications only invalidate inventory. Event data never leaves this module.
type Run = (command: string, args: string[]) => string;
export interface RigChange { available: boolean; changed: boolean }
const unavailable = (): RigChange => ({ available: false, changed: false });

export async function waitForRigChange(run: Run, fetcher: typeof fetch = fetch, waitMs = 20000, onReady: () => void = () => {}): Promise<RigChange> {
  let port: number;
  try {
    if (!['0.6.14 (df1c1517)', '0.7.18 (3bf87c62)'].includes(run('rig', ['--version']).trim())) return unavailable();
    port = Number(/running on port (\d+)/.exec(run('rig', ['daemon', 'status']))?.[1]);
    if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) return unavailable();
  } catch { return unavailable(); }
  // An explicit different endpoint must not silently subscribe to a local daemon.
  const endpoint = process.env.OPENRIG_URL;
  if (endpoint && endpoint !== `http://127.0.0.1:${port}` && endpoint !== `http://localhost:${port}`) return unavailable();
  const controller = new AbortController();
  let expired = false;
  let connected = false;
  const timer = setTimeout(() => { expired = true; controller.abort(); }, Math.min(20000, Math.max(1, waitMs)));
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  try {
    // No Last-Event-ID: reconnecting deliberately starts live, never replaying history.
    const response = await fetcher(`http://127.0.0.1:${port}/api/events?replay=0`, {
      method: 'GET', headers: { Accept: 'text/event-stream' }, redirect: 'error', signal: controller.signal,
    });
    if (!response.ok || !response.headers.get('content-type')?.startsWith('text/event-stream') || !response.body) {
      await response.body?.cancel(); return unavailable();
    }
    connected = true;
    onReady();
    reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8', { fatal: true });
    let buffer = ''; let bytes = 0; let frames = 0;
    for (;;) {
      const part = await reader.read();
      if (part.done) return unavailable();
      bytes += part.value.byteLength;
      if (bytes > 256 * 1024) return unavailable();
      buffer += decoder.decode(part.value, { stream: true });
      // Normalize complete lines, including CRLF split across chunks.
      let end: number;
      while ((end = frameEnd(buffer)) >= 0) {
        const delimiter = buffer.startsWith('\r\n\r\n', end) ? 4 : 2;
        const frame = buffer.slice(0, end).replaceAll('\r\n', '\n');
        buffer = buffer.slice(end + delimiter);
        if (++frames > 128 || Buffer.byteLength(frame, 'utf8') > 64 * 1024) return unavailable();
        let id = ''; const data: string[] = [];
        for (const line of frame.split('\n')) {
          if (line.startsWith('id:')) id = line.slice(3).trim();
          if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
        }
        if (!id && !data.length) continue; // bounded keepalive comments
        if (!/^[1-9][0-9]{0,15}$/.test(id) || !Number.isSafeInteger(Number(id))) return unavailable();
        const value = JSON.parse(data.join('\n')) as { seq?: unknown } | null;
        if (!value || typeof value !== 'object' || Array.isArray(value) || value.seq !== Number(id)) return unavailable();
        return { available: true, changed: true };
      }
      if (Buffer.byteLength(buffer, 'utf8') > 64 * 1024) return unavailable();
    }
  } catch { return expired && connected ? { available: true, changed: false } : unavailable(); }
  finally {
    clearTimeout(timer);
    controller.abort();
    await reader?.cancel().catch(() => {});
    reader?.releaseLock();
  }
}

function frameEnd(buffer: string): number {
  const lf = buffer.indexOf('\n\n'); const crlf = buffer.indexOf('\r\n\r\n');
  return lf < 0 ? crlf : crlf < 0 ? lf : Math.min(lf, crlf);
}
