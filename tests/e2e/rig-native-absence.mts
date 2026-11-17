export interface NativeAbsenceProof {
  session: string;
  pane: string;
  observedAt: string;
}

/** An older enrollment's missing-pane sample cannot acknowledge this kill. */
export function nativeAbsenceProof(value: unknown, session: string, pane: string, after: number): NativeAbsenceProof | null {
  if (!Array.isArray(value)) return null;
  const rows = value.filter((row: unknown): row is Record<string, unknown> => Boolean(row && typeof row === 'object'
    && !Array.isArray(row) && (row as Record<string, unknown>).canonicalSessionName === session));
  if (rows.length !== 1) return null;
  const row = rows[0];
  if (row.sessionStatus !== 'detached' || row.lifecycleState !== 'detached') return null;
  const identity = row.identityVerdict as Record<string, unknown> | null;
  if (!identity || identity.sessionName !== session || identity.verdict !== 'pane_missing' || identity.reason !== 'session_missing'
    || typeof identity.observedAt !== 'string' || !Number.isFinite(after)) return null;
  const time = Date.parse(identity.observedAt);
  const evidence = identity.evidence as Record<string, unknown> | null;
  if (!Number.isFinite(time) || time < after || !evidence || evidence.registeredPane !== pane) return null;
  return { session, pane, observedAt: identity.observedAt };
}
