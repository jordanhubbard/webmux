const frameTypes = new Set(['input', 'resize', 'output', 'status', 'focus', 'viewer_join', 'viewer_leave', 'error',
  'transcript_toggle', 'transcript_status', 'browser_open', 'browser_ack', 'clipboard', 'clipboard_read', 'clipboard_reply', 'input_error']);
const states = new Set(['connecting', 'connected', 'disconnected']);
const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);

// Never retain frame bodies, error messages, URLs, tokens or arbitrary names.
export function federationFrameFacts(payload: string | Buffer) {
  const bytes = Buffer.byteLength(payload);
  if (bytes > 65536) return { bytes, type: 'oversize' };
  let value: unknown;
  try { value = JSON.parse(String(payload)); } catch { return { bytes, type: 'non-json' }; }
  if (!object(value)) return { bytes, type: 'other' };
  return { bytes, type: typeof value.type === 'string' && frameTypes.has(value.type) ? value.type : 'other',
    ...(typeof value.state === 'string' && states.has(value.state) ? { state: value.state } : {}),
    viewer: typeof value.viewer_id === 'string', profile: typeof value.snapshot_profile === 'string',
    token: typeof value.snapshot_token === 'string', dataBytes: typeof value.data === 'string' ? Buffer.byteLength(value.data) : 0,
    ...(Number.isSafeInteger(value.cols) && Number(value.cols) > 0 && Number(value.cols) <= 1000 ? { cols: Number(value.cols) } : {}),
    ...(Number.isSafeInteger(value.rows) && Number(value.rows) > 0 && Number(value.rows) <= 1000 ? { rows: Number(value.rows) } : {}) };
}

export function federationRouteFacts(address: string) {
  try {
    const pathname = new URL(address).pathname;
    if (/^\/api\/term\/[a-zA-Z0-9_-]{1,128}$/.test(pathname)) return 'direct';
    const remote = /^\/api\/servers\/([a-zA-Z0-9_-]{1,128})\/term\/[a-zA-Z0-9_-]{1,128}$/.exec(pathname);
    if (remote) return remote[1] === 'local' ? 'federated-local' : 'federated-peer';
  } catch { /* Not a recognized fixture terminal URL. */ }
  return undefined;
}

