// Read-only manager-reported enrollment lineage. It does not authorize Stop,
// establish provider liveness, or infer lifecycle ownership of an SSH destination.
export interface EnrollmentSeat { rigId?: string; logicalId?: string; canonicalSessionName?: string }
export interface RigEnrollment {
  enrollmentOrigin: 'launched' | 'claimed' | 'unknown';
  enrollmentSource: 'rig-session-registry' | 'unavailable';
  enrollmentId?: string;
  enrollmentObservedAt?: string;
}
type Row = Record<string, unknown>;
type Run = (command: string, args: string[]) => string;
const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const maxBytes = 512 * 1024;
const unknown = (): RigEnrollment => ({ enrollmentOrigin: 'unknown', enrollmentSource: 'unavailable' });
const object = (value: unknown): value is Row => !!value && typeof value === 'object' && !Array.isArray(value);
const rows = (value: unknown): Row[] => {
  if (!Array.isArray(value) || value.length > 4096 || !value.every(object)) throw new Error('Enrollment records unavailable');
  return value;
};

export async function observeRigEnrollments(run: Run, seats: EnrollmentSeat[], fetcher: typeof fetch = fetch): Promise<RigEnrollment[]> {
  const result = seats.map(unknown);
  if (!seats.length || seats.length > 256) return result;
  let port: number;
  try {
    // This lineage join follows the qualified inventory's greatest-session-ULID
    // rule. Other builds degrade, independently of attach and native recovery.
    if (!['0.6.14 (df1c1517)', '0.7.18 (3bf87c62)'].includes(run('rig', ['--version']).trim())) return result;
    port = Number(/running on port (\d+)/.exec(run('rig', ['daemon', 'status']))?.[1]);
    if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) return result;
  } catch { return result; }
  const rigIds = [...new Set(seats.map(seat => seat.rigId).filter((id): id is string => typeof id === 'string' && ulid.test(id)))];
  if (rigIds.length > 32) return result;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  let remainingBytes = 8 * 1024 * 1024;
  const get = async (route: string): Promise<Row[]> => {
    const response = await fetcher(`http://127.0.0.1:${port}${route}`, { method: 'GET', redirect: 'error', signal: controller.signal });
    if (!response.ok || !response.body || Number(response.headers.get('content-length') || 0) > maxBytes) { await response.body?.cancel(); throw new Error('Enrollment records unavailable'); }
    const reader = response.body.getReader();
    const chunks: Uint8Array[] = [];
    let size = 0;
    try {
      for (;;) {
        const part = await reader.read();
        if (part.done) break;
        size += part.value.length;
        remainingBytes -= part.value.length;
        if (size > maxBytes || remainingBytes < 0) { await reader.cancel(); if (remainingBytes < 0) controller.abort(); throw new Error('Enrollment records exceed limit'); }
        chunks.push(part.value);
      }
      return rows(JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown);
    } finally { reader.releaseLock(); }
  };
  let cursor = 0;
  const worker = async () => {
    while (cursor < rigIds.length && !controller.signal.aborted) {
      const rigId = rigIds[cursor++];
      try {
        const reads = await Promise.allSettled([get(`/api/rigs/${rigId}/nodes`), get(`/api/rigs/${rigId}/sessions`)]);
        if (reads[0].status !== 'fulfilled' || reads[1].status !== 'fulfilled') continue;
        const [nodes, sessions] = [reads[0].value, reads[1].value];
        const observedAt = new Date().toISOString();
        for (const [index, seat] of seats.entries()) {
          if (seat.rigId !== rigId || !seat.logicalId || !seat.canonicalSessionName) continue;
          if (seats.filter(other => other.rigId === rigId && other.logicalId === seat.logicalId).length !== 1
            || seats.filter(other => other.canonicalSessionName === seat.canonicalSessionName).length !== 1) continue;
          const candidates = nodes.filter(node => node.rigId === rigId && node.logicalId === seat.logicalId);
          if (candidates.length !== 1) continue;
          const node = candidates[0];
          if (typeof node.nodeId !== 'string' || !ulid.test(node.nodeId) || node.canonicalSessionName !== seat.canonicalSessionName) continue;
          const registered = sessions.filter(session => session.nodeId === node.nodeId);
          // Never ignore a malformed row and then accidentally select history.
          if (!registered.length || registered.some(session => typeof session.id !== 'string' || !ulid.test(session.id))) continue;
          const latest = registered.reduce((left, right) => String(left.id) > String(right.id) ? left : right);
          if (registered.filter(session => session.id === latest.id).length !== 1 || latest.sessionName !== seat.canonicalSessionName
            || (node.storedSessionStatus !== undefined && node.storedSessionStatus !== latest.status)
            || (latest.origin !== 'launched' && latest.origin !== 'claimed') || controller.signal.aborted) continue;
          result[index] = { enrollmentOrigin: latest.origin, enrollmentSource: 'rig-session-registry', enrollmentId: String(latest.id), enrollmentObservedAt: observedAt };
        }
      } catch { /* Discovery and unrelated rigs remain usable; ownership is unknown. */ }
    }
  };
  try { await Promise.all([worker(), worker()]); }
  finally { clearTimeout(timer); controller.abort(); }
  return result;
}
