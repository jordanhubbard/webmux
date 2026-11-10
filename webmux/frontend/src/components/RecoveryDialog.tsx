import { useEffect, useState } from 'react';
import { api } from '../utils/api';
import type { RecoveryRequest, SessionRecovery } from '../types';
import { Dialog } from './Dialog';
import './RecoveryDialog.css';

/** Recorded history stays plain text and is loaded only on an explicit click. */
export function RecoveryDialog({ sessionId, onClose }: { sessionId: string; onClose: () => void }) {
  const [value, setValue] = useState<SessionRecovery | null>(null);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [approveReplay, setApproveReplay] = useState(false);
  const [approveManaged, setApproveManaged] = useState(false);
  const [shareContext, setShareContext] = useState(false);
  const [context, setContext] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    setValue(null); setError('');
    setApproveReplay(false); setApproveManaged(false); setShareContext(false); setContext('');
    api.getSessionRecovery(sessionId, controller.signal).then(result => {
      if (!controller.signal.aborted) setValue(result);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Cannot load recorded work');
    });
    return () => controller.abort();
  }, [sessionId, refresh]);
  const managerName = value?.lifecycle_owner === 'rig' ? 'Rig' : 'Agent Deck';
  const activity = value?.state?.activity;
  const recordedRoots = Object.entries(activity?.configuration?.roots ?? {}).filter(([name]) => name !== 'HOME');
  const refreshPreview = () => { setApproveReplay(false); setApproveManaged(false); setShareContext(false); setContext(''); setRefresh(count => count + 1); };
  const perform = async (kind: RecoveryRequest['kind']) => {
    if (!value?.state || !value.instance || value.host_revision == null || busy) return;
    if (kind === 'managed' ? !value.can_resume_managed || !approveManaged : !value.waiting) return;
    setBusy(true); setActionError('');
    try {
      const selected = kind === 'native' && shareContext ? context : '';
      if (new TextEncoder().encode(selected).length > 16 * 1024) throw new Error('Select at most 16 KiB of output to share.');
      const token = crypto.getRandomValues(new Uint8Array(16));
      token[6] = (token[6] & 15) | 64; token[8] = (token[8] & 63) | 128;
      const hex = Array.from(token, byte => byte.toString(16).padStart(2, '0')).join('');
      const id = `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
      await api.recoverSession(sessionId, { id, kind, revision: value.state.revision,
        host_revision: value.host_revision, instance: value.instance,
        confirm: kind === 'managed' ? approveManaged : kind === 'replay' ? approveReplay : shareContext, ...(selected ? { context: selected } : {}) });
      onClose();
    } catch (cause) { setActionError(cause instanceof Error ? cause.message : 'Cannot continue this work'); refreshPreview(); }
    finally { setBusy(false); }
  };
  const setAutomatic = async (enabled: boolean) => {
    setBusy(true); setActionError('');
    try { await api.setAutoRecovery(sessionId, enabled); refreshPreview(); }
    catch (cause) { setActionError(cause instanceof Error ? cause.message : 'Cannot save recovery preference'); }
    finally { setBusy(false); }
  };
  return <Dialog title="Recover work" onClose={onClose} dismissible={!busy} width={680}>
    <div className="recovery-content">
      {error && <p role="alert">{error}</p>}
      {actionError && <p role="alert">{actionError}</p>}
      {!value && !error && <p role="status">Loading recorded work…</p>}
      {value && <>
        <p>{value.message}</p>
        {value.warning && <p role="status">{value.warning}</p>}
        <dl className="recovery-details">
          <dt>Execution host</dt><dd>{value.hostname}{!value.manager_target && ` · ${value.transport.toUpperCase()}`}</dd>
          <dt>Session manager</dt><dd>{value.lifecycle_owner === 'webmux' ? 'WebMux' : value.lifecycle_owner}</dd>
          {value.manager_target && <><dt>Managed session</dt><dd>{value.manager_target}</dd></>}
          {value.manager_directory && <><dt>Managed project directory</dt><dd>{value.manager_directory}</dd></>}
          {value.manager_snapshot_id && <><dt>Recorded manager checkpoint</dt><dd>{value.manager_snapshot_id}</dd></>}
          {value.manager_profile && <><dt>Manager profile</dt><dd>{value.manager_profile}</dd></>}
          {value.manager_native_session && <>
            <dt>Recorded managed conversation</dt><dd>{value.manager_native_session}</dd>
            <dt>Conversation evidence</dt><dd>Recorded from the lifecycle manager. Later conversation changes have not been verified.</dd>
          </>}
          {value.manager_native_state_file && <><dt>Managed native state file</dt><dd>{value.manager_native_state_file}</dd></>}
          {value.manager_native_observed_at && <><dt>Managed conversation observation</dt><dd>{new Date(value.manager_native_observed_at).toLocaleString()}</dd></>}
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
          <dt>Conversation evidence</dt><dd>{activity.native_evidence === 'process-state'
            ? 'Observed in the running CLI process’s native state.'
            : activity.native_evidence === 'native-hook'
              ? 'Reported by the coding CLI when the conversation started. Hooks can stop reporting later changes; automatic resume is unavailable.'
            : activity.native_evidence === 'launch-selector'
              ? 'Selected at launch. Later conversation changes have not been verified; automatic resume is unavailable.'
              : 'The current conversation could not be verified; automatic resume is unavailable.'}</dd>
          {activity.native_state_file && <><dt>Native state file</dt><dd>{activity.native_state_file}</dd></>}
          {recordedRoots.length > 0 && <><dt>Original CLI paths</dt><dd>{recordedRoots.map(([name, path]) => <div key={name}>{name}: {path || '(empty)'}</div>)}</dd></>}
        </dl>}
        {value.history_available ? <>
          <h3>Previous terminal output</h3>
          <pre className="recovery-history" tabIndex={0} aria-label="Previous terminal output">{value.history}</pre>
        </> : <p>No retained terminal output is available. Existing native conversation information is shown above when recorded.</p>}
        {!activity && !value.can_resume_managed && <p>{value.manager_native_session ? 'Resume this recorded conversation through its session manager. Retained terminal output is available below when recorded.' : 'No reliable command was recorded. Use any retained output to recover this session manually.'}</p>}
        {value.can_resume_managed && <>
          <p>{managerName} will verify the recorded destination and policy, then reattach a surviving process or resume the recorded conversation. No terminal output will be sent to the coding service.</p>
          <label className="recovery-consent"><input type="checkbox" checked={approveManaged} disabled={busy}
            onChange={event => setApproveManaged(event.target.checked)} />
            I approve asking {managerName} to resume {value.manager_native_session} on {value.hostname} in {value.manager_directory}
          </label>
          <div className="recovery-actions"><button type="button" disabled={busy || !approveManaged}
            onClick={() => void perform('managed')}>Resume through {managerName}</button></div>
        </>}
        {value.lifecycle_owner === 'webmux' && <label className="recovery-consent">
          <input type="checkbox" checked={value.state?.auto_resume ?? false} disabled={busy}
            onChange={event => void setAutomatic(event.target.checked)} />
          Automatically resume interrupted native coding conversations while session logging is enabled
        </label>}
        {value.waiting && <>
          {value.can_resume_cli && value.history_available && <>
            <label className="recovery-consent"><input type="checkbox" checked={shareContext} disabled={busy}
              onChange={event => {
                setShareContext(event.target.checked);
                if (event.target.checked && !context) setContext(new TextDecoder().decode(new TextEncoder().encode(value.history).slice(-(16 * 1024 - 4))));
              }} />
              I approve sharing the selected terminal output with {activity?.cli} when resuming
            </label>
            {shareContext && <textarea aria-label="Selected terminal output to share" value={context} disabled={busy}
              onChange={event => setContext(event.target.value)} rows={6} />}
          </>}
          {value.can_replay && <label className="recovery-consent"><input type="checkbox" checked={approveReplay} disabled={busy}
            onChange={event => setApproveReplay(event.target.checked)} />
            I approve running the displayed command again on {value.hostname} in {activity?.cwd}
          </label>}
          <div className="recovery-actions">
            {value.can_resume_cli && <button type="button" disabled={busy} onClick={() => void perform('native')}>Resume {activity?.cli} conversation</button>}
            {value.can_replay && <button type="button" disabled={busy || !approveReplay} onClick={() => void perform('replay')}>Run recorded command</button>}
            <button type="button" disabled={busy} onClick={() => void perform('shell')}>Start a shell</button>
          </div>
        </>}
      </>}
      <div className="recovery-actions">
        <button type="button" disabled={busy} onClick={() => { setActionError(''); refreshPreview(); }}>Refresh</button>
        <button type="button" disabled={busy} onClick={onClose}>Close</button>
      </div>
      {busy && <p role="status">Contacting the execution host…</p>}
    </div>
  </Dialog>;
}
