import { useCallback, useEffect, useState } from 'react';
import { api, type MacSetup, type PermissionResult } from '../utils/api';
import { Dialog } from './Dialog';
import './MacPermissions.css';

const labels: Record<string, string> = {
  unchecked: 'Not checked', allowed: 'Access verified', denied: 'Permission denied',
  waiting: 'Still waiting for macOS', unavailable: 'Access could not be verified',
  missing: 'Folder does not exist', error: 'Check could not complete',
};
function resolved(result: PermissionResult) {
  return result.state !== 'waiting' && result.folders?.length === 3 && result.folders.every(folder => ['allowed', 'denied', 'missing'].includes(folder.state));
}
function Result({ result }: { result: PermissionResult }) {
  return <div role="status">
    <strong>{labels[result.state] ?? result.state}</strong>
    {result.detail && <p>{result.detail}</p>}
    {result.folders && <ul>{result.folders.filter(folder => !['allowed', 'denied', 'missing'].includes(folder.state)).map(folder => <li key={folder.name}>{folder.name}: {labels[folder.state] ?? folder.state}</li>)}</ul>}
    {result.checked_at && <small>Checked {new Date(result.checked_at).toLocaleString()}</small>}
  </div>;
}

export function MacPermissions() {
  const [setup, setSetup] = useState<MacSetup | null>(null);
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [restart, setRestart] = useState(false);
  const [restartInstance, setRestartInstance] = useState<string | null>(null);
  const load = useCallback(async () => {
    const value = await api.getMacSetup();
    setSetup(value);
    return value;
  }, []);
  useEffect(() => {
    let active = true;
    api.getMacSetup().then(value => {
      if (!active || !value.status.supported) return;
      setSetup(value);
      setVisible(!value.skipped && ['unchecked', 'waiting'].includes(value.status.local.state));
    }).catch(() => { /* Mac setup is restricted to administrators and local unauthenticated users. */ });
    return () => { active = false; };
  }, []);

  const localState = setup?.status.local.state;
  // Poll status only: starting a new probe would repeat prompts and flicker the UI.
  useEffect(() => {
    if (!visible || localState !== 'waiting') return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const value = await api.getMacSetup();
        if (cancelled) return;
        setSetup(value);
        if (value.status.local.state !== 'waiting') {
          if (resolved(value.status.local)) {
            setVisible(false);
            // Remember completion, including explicit denials, across restarts.
            await api.macSetupAction('skip');
          }
          return;
        }
      } catch { /* Retain the current view during transient connection failures. */ }
      if (!cancelled) timer = setTimeout(() => { void poll(); }, 750);
    };
    timer = setTimeout(() => { void poll(); }, 750);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [visible, localState]);

  // Poll for the observable end of restart. Never assume elapsed time means success.
  useEffect(() => {
    if (!restart) return;
    const deadline = Date.now() + 30000;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const value = await api.getMacSetup();
        // Exec keeps the PID; compare a fresh process instance instead.
        if (!cancelled && value.status.instance !== restartInstance) {
          setSetup(value); setRestart(false); setBusy(null);
          setNotice('WebMux restarted. Run both checks again, then retry in a fresh terminal. Existing tmux shells were preserved.');
          return;
        }
      } catch { /* The server is replacing its process. */ }
      if (cancelled) return;
      if (Date.now() >= deadline) {
        setRestart(false); setBusy(null); setError('Could not confirm the restart. Check your WebMux service and reload this page.');
      } else { timer = setTimeout(() => { void poll(); }, 750); }
    };
    timer = setTimeout(() => { void poll(); }, 750);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [restart, restartInstance]);

  if (!setup?.status.supported) return null;
  const { status } = setup;
  const action = async (name: string, transport?: 'local' | 'ssh') => {
    setBusy(transport ?? name); setError(null); setNotice(null);
    try {
      await api.macSetupAction(name, transport);
      if (name === 'restart') { setRestartInstance(status.instance); setRestart(true); return; }
      const value = await load();
      if (name === 'check' && transport === 'local' && resolved(value.status.local)) {
        setVisible(false);
        await api.macSetupAction('skip');
      }
      if (name === 'skip') setVisible(false);
    } catch (err) { setError(err instanceof Error ? err.message : String(err)); }
    setBusy(null);
  };
  const openButton = (name: string, text: string) => <button type="button" disabled={!!busy || !status.can_open_settings}
    title={status.can_open_settings ? undefined : `Open System Settings directly on ${status.hostname}`}
    onClick={() => { void action(name); }}>{text}</button>;
  const ready = status.local.state === 'allowed';
  return <>
    <div className="mac-permissions-banner">
      <span>{ready ? 'Mac local folder access verified' : 'Mac folder access needs setup'}</span>
      <button type="button" onClick={() => setVisible(true)}>Mac permissions</button>
    </div>
    {visible && <Dialog title="Mac permissions" subtitle={`Complete these steps on ${status.hostname}, the Mac running WebMux.`}
      onClose={() => { if (!busy) void action('skip'); }} dismissible={!busy} width={720}>
      <div className="mac-permissions-body">
        <p>macOS protects Desktop, Documents, and Downloads. You approve access in System Settings; WebMux cannot grant itself permission. You can skip this step if you do not need these folders.</p>
        {!status.can_open_settings && <p>These settings are on <strong>{status.hostname}</strong>. Use its desktop or Screen Sharing. Buttons that open its desktop are available when you visit WebMux directly on that Mac.</p>}
        <h3>1. Allow local terminals to access your files</h3>
        <p>Open <strong>System Settings → Privacy &amp; Security → Full Disk Access</strong>. Click <strong>+</strong>, select the WebMux item below, and enable its switch. Full Disk Access also covers other protected files accessible to your account.</p>
        <p style={{ overflowWrap: 'anywhere', userSelect: 'all' }}><code>{status.permission_target}</code></p>
        <p>In the file picker, press <strong>⌘⇧G</strong> and paste that path. “Show WebMux in Finder” reveals the item to select.</p>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>{openButton('privacy', 'Open Privacy settings')}{openButton('reveal', 'Show WebMux in Finder')}
          <button disabled={!!busy || status.local.state === 'waiting'} onClick={() => { void action('check', 'local'); }}>{busy === 'local' || status.local.state === 'waiting' ? 'Checking…' : 'Check local access'}</button></div>
        <Result result={status.local} />
        <h3>2. Allow SSH access, if you use SSH to this Mac</h3>
        <p>Open <strong>System Settings → General → Sharing → Remote Login ⓘ</strong>. Enable <strong>Allow full disk access for remote users</strong>. This applies to SSH users allowed by Remote Login, separately from local terminals.</p>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>{openButton('sharing', 'Open Sharing settings')}
          <button disabled={!!busy} onClick={() => { void action('check', 'ssh'); }}>{busy === 'ssh' ? 'Checking…' : 'Check localhost SSH access'}</button></div>
        <p><small>The automated SSH check uses your Mac account and default SSH identity on localhost:22. Other accounts, keys, or ports must be checked in their own SSH terminal.</small></p>
        <Result result={status.ssh} />
        <h3>3. Restart and retest</h3>
        <p>After changing permissions, restart WebMux, run the checks again, and retry in a fresh local or SSH terminal. Existing tmux shells stay running; this does not kill the tmux server.</p>
        <button disabled={!!busy} onClick={() => { void action('restart'); }}>{restart ? 'Restarting WebMux…' : 'Restart WebMux and reconnect'}</button>
        {notice && <p role="status">{notice}</p>}
        {error && <p role="alert" style={{ color: 'var(--danger, #ff8080)' }}>{error}</p>}
        <details><summary>Diagnostic details</summary><pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', userSelect: 'all' }}>{JSON.stringify(setup, null, 2)}</pre></details>
        <p><button disabled={!!busy} onClick={() => { void action('skip'); }}>{ready ? 'Done' : 'Skip for now'}</button> You can reopen Mac permissions from the banner above your workspace.</p>
      </div>
    </Dialog>}
  </>;
}
