import { useEffect, useState } from 'react';
import { api } from '../utils/api';
import type { SessionRecovery } from '../types';
import { Dialog } from './Dialog';
import './RecoveryDialog.css';

/** Recorded history stays plain text and is loaded only on an explicit click. */
export function RecoveryDialog({ sessionId, onClose }: { sessionId: string; onClose: () => void }) {
  const [value, setValue] = useState<SessionRecovery | null>(null);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setValue(null); setError('');
    api.getSessionRecovery(sessionId, controller.signal).then(result => {
      if (!controller.signal.aborted) setValue(result);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Cannot load recorded work');
    });
    return () => controller.abort();
  }, [sessionId, refresh]);
  const activity = value?.state?.activity;
  return <Dialog title="Recover work" onClose={onClose} width={680}>
    <div className="recovery-content">
      {error && <p role="alert">{error}</p>}
      {!value && !error && <p role="status">Loading recorded work…</p>}
      {value && <>
        <p>{value.message}</p>
        <dl className="recovery-details">
          <dt>Execution host</dt><dd>{value.hostname} · {value.transport.toUpperCase()}</dd>
          <dt>Session manager</dt><dd>{value.lifecycle_owner === 'webmux' ? 'WebMux' : value.lifecycle_owner}</dd>
          {activity?.cwd && <><dt>Working directory</dt><dd>{activity.cwd}</dd></>}
          {activity?.observed_at && <><dt>Last observation</dt><dd>{new Date(activity.observed_at).toLocaleString()}</dd></>}
          {activity && <><dt>Evidence</dt><dd>{activity.source === 'cli-native-state' ? 'Native coding CLI metadata' : 'Shell observation'}</dd></>}
        </dl>
        {activity?.command && <>
          <h3>Last observed command</h3>
          <pre className="recovery-command">{activity.command}</pre>
        </>}
        {activity?.cli && <dl className="recovery-details">
          <dt>Coding CLI</dt><dd>{activity.cli} {activity.cli_version}</dd>
          <dt>Recorded native session</dt><dd>{activity.native_session_id || 'No reliable native session ID was recorded.'}</dd>
        </dl>}
        {value.history_available ? <>
          <h3>Previous terminal output</h3>
          <pre className="recovery-history" tabIndex={0} aria-label="Previous terminal output">{value.history}</pre>
        </> : <p>No retained terminal output is available. Existing native conversation information is shown above when recorded.</p>}
        {!activity && <p>No reliable command was recorded. Use any retained output to recover this session manually.</p>}
      </>}
      <div className="recovery-actions">
        <button type="button" data-dialog-autofocus onClick={() => setRefresh(count => count + 1)}>Refresh</button>
        <button type="button" onClick={onClose}>Close</button>
      </div>
    </div>
  </Dialog>;
}
