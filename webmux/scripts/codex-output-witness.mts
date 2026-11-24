// Fixture-only witness for native command stdout, independent of model prose.
type Row = { threadId: string; turnId: string; itemId: string; counter: number };
export class OutputWitness {
  private readonly nonces = new Map<string, string>();
  private readonly items = new Map<string, { tail: string; row: Row }>();
  constructor(watches: { thread: string; nonce: string }[]) {
    if (watches.length !== 4) throw new Error('Four fixture threads required');
    for (const watch of watches) {
      if (!/^[a-f0-9-]{36}$/.test(watch.thread) || !/^[a-f0-9]{32}$/.test(watch.nonce) || this.nonces.has(watch.thread)) throw new Error('Invalid fixture identity');
      this.nonces.set(watch.thread, watch.nonce);
    }
  }
  accept(message: Record<string, unknown>): boolean {
    if (message.method !== 'item/commandExecution/outputDelta') return false;
    const p = message.params as Partial<Row> & { delta?: unknown } | undefined;
    if (!p || typeof p.threadId !== 'string' || typeof p.turnId !== 'string' || typeof p.itemId !== 'string' || typeof p.delta !== 'string' || p.turnId.length > 512 || p.itemId.length > 512) return false;
    const nonce = this.nonces.get(p.threadId);
    if (!nonce) return false;
    const key = JSON.stringify([p.threadId, p.turnId, p.itemId]);
    if (!this.items.has(key) && this.items.size >= 64) throw new Error('Fixture command witness bound exceeded');
    const item = this.items.get(key) ?? { tail: '', row: { threadId: p.threadId, turnId: p.turnId, itemId: p.itemId, counter: 0 } };
    const text = item.tail + p.delta;
    const lines = text.split('\n');
    item.tail = (lines.pop() ?? '').slice(-65536);
    let advanced = false;
    for (const line of lines) {
      const match = new RegExp(`^WM_CODING_${nonce}_(\\d{1,6})\\r?$`).exec(line);
      if (match && Number(match[1]) > item.row.counter) { item.row.counter = Number(match[1]); advanced = true; }
    }
    this.items.set(key, item);
    return advanced;
  }
  rows(): Row[] { return [...this.items.values()].filter(item => item.row.counter > 0).map(item => ({ ...item.row })); }
}
