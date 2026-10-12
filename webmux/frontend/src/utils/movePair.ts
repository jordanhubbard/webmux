/** Compensate a partial swap, then let the caller reload authoritative state. */
export async function movePair(first: () => Promise<unknown>, second: (() => Promise<unknown>) | undefined,
  undoFirst: () => Promise<unknown>): Promise<void> {
  await first();
  if (!second) return;
  try { await second(); }
  catch (error) {
    try { await undoFirst(); }
    catch { throw new Error('Move partially saved; rollback failed. Reloading the saved layout.'); }
    throw error;
  }
}
