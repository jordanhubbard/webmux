import { useState } from 'react';
import { Dialog } from './Dialog';
import { api } from '../utils/api';
import type { ManagedClient, Session } from '../types';

export function ManagedHostDialog({ session, client, onClose, onChanged, onSessionUpdated }: {
  session: Session;
  client?: ManagedClient;
  onClose: () => void;
  onChanged: () => void;
  onSessionUpdated: (session: Session) => void;
}) {
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [serverURL, setServerURL] = useState(window.location.origin);
  const server = serverURL.trim().replace(/\/$/, '');
  const command = `webmux-client --server ${server}${server.startsWith('http://') ? ' --allow-insecure-http' : ''} --background`;
  const act = async (work: () => Promise<void>) => {
    setBusy(true); setError(''); setMessage('');
    try { await work(); onChanged(); }
    catch (reason) { setError(String(reason)); }
    finally { setBusy(false); }
  };
  return <Dialog title="Managed host" subtitle={`${session.username}@${session.hostname}`} onClose={onClose} dismissible={!busy} width={560}>
    <div style={{ padding: 16 }}>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      {!session.host_id && <>
        <p>Save this SSH destination to associate its terminal windows with a client. The saved host must use this address, account and port.</p>
        <button type="button" disabled={busy} onClick={() => void act(async () => {
          const host = await api.createHost({ hostname: session.hostname, port: session.port || 22, username: session.username, transport: 'ssh' });
          const updated = await api.associateSessionHost(session.id, host.id);
          onSessionUpdated(updated); setMessage('Host saved. Enter the client pairing code below.');
        })}>Save this host</button>
      </>}
      {session.host_id && !client && <>
        <p>Install and start <strong>webmux-client</strong> on this host. It connects outward to WebMux and prints a one-time pairing code.</p>
        <label>WebMux URL reachable from this host <input type="url" value={serverURL} onChange={event => setServerURL(event.target.value)} style={{ width: '100%' }} /></label>
        {/^https?:\/\/[^\s/]+$/.test(server) && <code style={{ display: 'block', overflowWrap: 'anywhere', padding: 8, background: '#17172b' }}>{command}</code>}
        {server.startsWith('http://') && <p>HTTP exposes login credentials on the network. Use HTTPS outside a trusted network.</p>}
        <form onSubmit={event => { event.preventDefault(); void act(async () => {
          await api.bindClient(session.host_id, code.trim().toUpperCase());
          setMessage('Client linked to this host.'); setCode('');
        }); }}>
          <label>Pairing code <input data-dialog-autofocus value={code} onChange={event => setCode(event.target.value)} maxLength={16} autoComplete="off" required /></label>{' '}
          <button type="submit" disabled={busy || code.trim().length !== 16}>Link client</button>
        </form>
      </>}
      {client && <>
        <p><strong>{client.online ? 'Connected' : 'Offline'}</strong> · {client.label}</p>
        <dl>
          <div><dt>Remote host</dt><dd>{client.snapshot?.hostname || 'Unknown'}</dd></div>
          <div><dt>System</dt><dd>{client.snapshot ? `${client.snapshot.os} / ${client.snapshot.arch}` : 'Unknown'}</dd></div>
          <div><dt>CPUs</dt><dd>{client.snapshot?.cpus ?? 'Unknown'}</dd></div>
          <div><dt>1-minute load</dt><dd>{client.snapshot?.load1 ?? 'Unavailable'}</dd></div>
          <div><dt>Last seen</dt><dd>{client.last_seen ? new Date(client.last_seen).toLocaleString() : 'Never'}</dd></div>
        </dl>
        <button type="button" disabled={!client.online || busy} onClick={() => void act(async () => { await api.refreshClient(client.id); setMessage('Host facts refreshed.'); })}>Refresh host facts</button>{' '}
        <button type="button" disabled={busy} onClick={() => void act(async () => { await api.unbindClient(session.host_id); setMessage('Client unlinked. It remains registered.'); })}>Unlink host</button>{' '}
        <button type="button" disabled={busy} onClick={() => { if (window.confirm(`Revoke ${client.label}? Its connection will close and its stored credential will stop working.`)) void act(async () => { await api.revokeClient(client.id); setMessage('Client revoked.'); }); }}>Revoke client</button>
      </>}
    </div>
  </Dialog>;
}
