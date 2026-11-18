import { useState } from 'react';
import { Dialog } from './Dialog';
import { api } from '../utils/api';
import type { CoordinationSeat } from './CoordinationDialog';

export interface RigStopPreview {
  id: string; target: string; reason: string; expiresAt: string;
  project: string; seat: string; sessionName: string; runtime: string;
  tmuxServer: string; nodeId: string; sessionId: string; effect: string;
}
export interface RigStopReceipt {
  id: string; state: 'stopped' | 'refused' | 'uncertain'; code: string;
  posted?: boolean; stopPhase: 'none' | 'detach_attempted' | 'detached' | 'kill_attempted' | 'unknown';
}
function receiptMessage(receipt: RigStopReceipt) {
  if (receipt.state === 'stopped') return 'Rig confirmed that the reviewed local seat stopped and its viewers were disconnected. Viewer tiles and retained logs remain available.';
  if (receipt.state === 'refused') return 'Rig refused this request before a terminal effect. Nothing was stopped. Review the current seat before another request.';
  const phase = receipt.stopPhase === 'detach_attempted' ? 'Rig attempted to disconnect viewers; no kill was reported.'
    : receipt.stopPhase === 'detached' ? 'Rig disconnected viewers; no kill was reported.'
      : receipt.stopPhase === 'kill_attempted' ? 'Rig attempted seat termination, but its final state is unconfirmed.'
        : 'The Stop outcome is unconfirmed.';
  return `${phase} Inspect the seat through Rig before further lifecycle actions. This request will not be retried.`;
}

export function RigStopDialog({ seat, onClose, onComplete }: { seat: CoordinationSeat; onClose: () => void; onComplete: () => void }) {
  const [reason, setReason] = useState('');
  const [preview, setPreview] = useState<RigStopPreview | null>(null);
  const [receipt, setReceipt] = useState<RigStopReceipt | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const review = async () => {
    setBusy(true); setError('');
    try { setPreview(await api.previewRigStop(seat.id, reason)); setConfirmed(false); }
    catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const stop = async () => {
    if (!preview || !confirmed || busy || receipt) return;
    setBusy(true); setError('');
    try { setReceipt(await api.executeRigStop(preview.id)); }
    catch {
      setReceipt({ id: preview.id, state: 'uncertain', code: 'response_unavailable', stopPhase: 'unknown' });
    } finally { setBusy(false); onComplete(); }
  };
  return <Dialog title="Stop local Rig seat" subtitle={`${seat.projectName} / ${seat.name}`} width={620} onClose={onClose} dismissible={!busy}>
    <p>This ends the manager-owned local tmux seat. Hiding a viewer leaves its work running; Stop is a separate lifecycle action.</p>
    {!preview && !receipt && <form onSubmit={event => { event.preventDefault(); void review(); }}>
      <label>Reason for stopping<input required maxLength={1024} value={reason} onChange={event => setReason(event.target.value)} /></label>
      <p><button type="submit" disabled={busy || !reason.trim()}>{busy ? 'Checking Rig…' : 'Review local Stop'}</button></p>
    </form>}
    {error && <p role="alert">{error}</p>}
    {preview && !receipt && <>
      <p>{preview.effect}</p>
      <dl><dt>Seat</dt><dd>{preview.seat}</dd><dt>Runtime</dt><dd>{preview.runtime}</dd><dt>Reason</dt><dd>{preview.reason}</dd></dl>
      <details><summary>Reviewed destination</summary><dl>
        <dt>Rig</dt><dd><code>{preview.project}</code></dd><dt>Local tmux session</dt><dd><code>{preview.sessionName}</code></dd>
        <dt>Local tmux server</dt><dd><code>{preview.tmuxServer}</code></dd><dt>Native node</dt><dd><code>{preview.nodeId}</code></dd>
        <dt>Reviewed enrollment</dt><dd><code>{preview.sessionId}</code></dd>
      </dl></details>
      <p>This review expires at {new Date(preview.expiresAt).toLocaleTimeString()}. Rig will check the reviewed occupant again before acting.</p>
      <label><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} /> I understand this stops the local Rig seat and disconnects all its viewers.</label>
      <p><button type="button" disabled={busy || !confirmed} onClick={() => void stop()}>{busy ? 'Waiting for Rig…' : 'Confirm local seat Stop'}</button></p>
    </>}
    {receipt && <><p role="status">{receiptMessage(receipt)}</p><details><summary>Stop receipt</summary><p>{receipt.code} · {receipt.stopPhase}</p><code>{receipt.id}</code></details></>}
    {!busy && <p><button type="button" onClick={onClose}>{receipt ? 'Close' : 'Cancel'}</button></p>}
  </Dialog>;
}
