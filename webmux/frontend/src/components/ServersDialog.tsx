import { useEffect, useState } from 'react';
import { api, type ManagedServer, type ServerGrant, type ServerDirectory, type NodeDiscovery } from '../utils/api';
import { Dialog } from './Dialog';
import './ServersDialog.css';

type Editor = { coordination_reservations: boolean; coordination_messages: boolean; source_node_id: string; source_owner: string; coordination_owner: string; coordination: boolean; verified: boolean; kind: 'server' | 'grant'; existing: boolean; id: string; name: string; url: string; owner: string; allow_http: boolean; token_env: string; credential: string };
const emptyEditor = (kind: Editor['kind']): Editor => ({ coordination_reservations: false, coordination_messages: false, source_node_id: '', source_owner: '', coordination_owner: '', coordination: false, verified: false, kind, existing: false, id: '', name: '', url: '', owner: '', allow_http: false, token_env: '', credential: '' });

function serverIdentity(url: string, directory: ServerDirectory) {
  const parsed = new URL(url);
  const base = parsed.host
    .toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^[^a-z0-9]+|-+$/g, '').slice(0, 56) || 'server';
  const used = new Set(['local', 'all', ...directory.servers.map(server => server.id), ...directory.grants.map(grant => grant.id)]);
  let id = base;
  for (let suffix = 2; used.has(id); suffix++) id = `${base}-${suffix}`;
  return { id, name: parsed.host };
}

export function ServersDialog({ onClose, onChanged, currentUser, initiallyAddServer = false }: { initiallyAddServer?: boolean; onClose: () => void; onChanged: () => Promise<void>; currentUser: string | null }) {
  const [directory, setDirectory] = useState<ServerDirectory | null>(null);
  const [discovery, setDiscovery] = useState<NodeDiscovery | null>(null);
  const [discoveryError, setDiscoveryError] = useState('');
  const [editor, setEditor] = useState<Editor | null>(() => initiallyAddServer ? emptyEditor('server') : null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [showCredential, setShowCredential] = useState(false);
  useEffect(() => {
    let canceled = false;
    void api.getServerDirectory().then(value => { if (!canceled) setDirectory(value); }).catch(error => { if (!canceled) setError(String(error)); });
    const refreshDiscovery = () => { void api.getDiscoveredNodes().then(value => { if (!canceled) { setDiscovery(value); setDiscoveryError(''); } }).catch(() => { if (!canceled) setDiscoveryError('Cannot load network discovery. Manual connections remain available.'); }); };
    refreshDiscovery();
    const timer = window.setInterval(refreshDiscovery, 15000);
    return () => { canceled = true; window.clearInterval(timer); };
  }, []);
  const run = async (action: () => Promise<void>) => {
    setBusy(true); setError(''); setNotice('');
    try { await action(); } catch (error) { setError(String(error)); } finally { setBusy(false); }
  };
  const reload = async () => { setDirectory(await api.getServerDirectory()); await onChanged(); };
  const edit = (kind: Editor['kind'], entry?: ManagedServer | ServerGrant) => {
    setError(''); setNotice(''); setShowCredential(false);
    const peer = entry && 'url' in entry ? entry : undefined;
    setEditor({ ...emptyEditor(kind), owner: currentUser || 'anonymous', ...entry, coordination_reservations: !!entry?.coordination_reservations, coordination_messages: !!entry?.coordination_messages, source_node_id: entry && 'source_node_id' in entry ? entry.source_node_id ?? '' : '', source_owner: entry && 'source_owner' in entry ? entry.source_owner ?? '' : '', coordination_owner: peer?.coordination_owner ?? '', coordination: entry && 'coordination' in entry ? !!entry.coordination : false, verified: !!peer?.identity?.node_id, kind, existing: !!entry, credential: '' });
  };
  const remove = (kind: Editor['kind'], id: string) => {
    if (!window.confirm(kind === 'server' ? `Remove ${id}? Its terminal processes will keep running on that server.` : `Remove incoming grant ${id}? This revokes the other server’s access.`)) return;
    void run(async () => { if (kind === 'server') await api.deleteServer(id); else await api.deleteServerGrant(id); await reload(); });
  };
  return <Dialog title="Manage servers" subtitle="Manage your WebMux cluster: add, edit or remove server connections and incoming access." width={700} onClose={onClose} dismissible={!busy}>
    <div className="servers-manager">
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
      {editor ? <form onSubmit={event => {
        event.preventDefault();
        const input = editor;
        void run(async () => {
          if (input.kind === 'server') {
            if (!directory) throw new Error('Wait for the server directory to load, then retry.');
            const identity = serverIdentity(input.url, directory);
            await api.saveServer(input.existing ? input.id : identity.id, { name: input.name.trim() || identity.name, url: input.url, allow_http: input.allow_http, coordination_owner: input.coordination_owner, coordination_messages: input.coordination_messages, coordination_reservations: input.coordination_reservations, token_env: input.token_env, credential: input.credential });
          } else await api.saveServerGrant(input.id, { owner: input.owner, coordination: input.coordination, coordination_messages: input.coordination_messages, coordination_reservations: input.coordination_reservations, source_node_id: input.source_node_id, source_owner: input.source_owner, token_env: input.token_env, credential: input.credential });
          // Discard the transient credential immediately after a successful save.
          setEditor(null); setShowCredential(false);
          await reload(); setNotice('Saved. Credentials stay on this server and are never returned to the browser.');
        });
      }}>
        <h3>{editor.existing ? 'Edit' : 'Add'} {editor.kind === 'server' ? 'server' : 'incoming grant'}</h3>
        <fieldset disabled={busy}>
          {editor.kind === 'server' ? <>
            <label>Server URL<input data-dialog-autofocus required type="url" placeholder="https://builder.example.net:8443" value={editor.url} onChange={e => setEditor({ ...editor, url: e.target.value })} /></label>
            <label>Name (optional)<input placeholder="Defaults to the server address" value={editor.name} onChange={e => setEditor({ ...editor, name: e.target.value })} /></label>
            <label className="servers-check"><input type="checkbox" checked={editor.allow_http} onChange={e => setEditor({ ...editor, allow_http: e.target.checked })} />Allow HTTP for a trusted private network or tunnel</label>
            {editor.verified ? <><label>Coordination account on this server<input placeholder="Leave empty to disable" value={editor.coordination_owner} onChange={e => setEditor({ ...editor, coordination_owner: e.target.value, coordination_reservations: e.target.value ? editor.coordination_reservations : false, coordination_messages: e.target.value ? editor.coordination_messages : false })} /></label><p>This local account may discover same-project sessions and claims from the verified receiving account. Its incoming grant must also allow project coordination summaries. Messages require separate permission on both servers.</p><label className="servers-check"><input type="checkbox" disabled={!editor.coordination_owner} checked={editor.coordination_messages} onChange={e => setEditor({ ...editor, coordination_messages: e.target.checked })} />Allow coordination messages to this peer</label><p>Messages enter the recipient’s coordination inbox. They do not type into terminals or start work.</p><label className="servers-check"><input type="checkbox" disabled={!editor.coordination_owner} checked={editor.coordination_reservations} onChange={e => setEditor({ ...editor, coordination_reservations: e.target.checked })} />Allow shared worker reservations on this peer</label><p>Eligible sessions may reserve this peer’s heavy worker. Its incoming grant must separately permit reservations. A reservation does not start a job.</p></> : <p>Verify the HTTPS node identity after saving to enable project coordination summaries.</p>}
          </> : <>
            <label>Grant name<input data-dialog-autofocus required pattern="[A-Za-z0-9][A-Za-z0-9_-]{0,63}" maxLength={64} disabled={editor.existing} value={editor.id} onChange={e => setEditor({ ...editor, id: e.target.value })} /></label>
            <label>Session owner<input required value={editor.owner} onChange={e => setEditor({ ...editor, owner: e.target.value })} /></label>
            <label className="servers-check"><input type="checkbox" checked={editor.coordination} onChange={e => setEditor({ ...editor, coordination: e.target.checked, coordination_reservations: e.target.checked && editor.coordination_reservations, coordination_messages: e.target.checked && editor.coordination_messages })} />Allow project coordination summaries</label>
            <p>When enabled, this trusted server may read this account’s verified project peers, claims and worker availability over HTTPS. Inbox messages and credentials are not shared in summaries.</p><label className="servers-check"><input type="checkbox" disabled={!editor.coordination} checked={editor.coordination_messages} onChange={e => setEditor({ ...editor, coordination_messages: e.target.checked })} />Allow incoming coordination messages</label><label className="servers-check"><input type="checkbox" disabled={!editor.coordination} checked={editor.coordination_reservations} onChange={e => setEditor({ ...editor, coordination_reservations: e.target.checked })} />Allow incoming shared worker reservations</label>{(editor.coordination_messages || editor.coordination_reservations) && <><label>Sending node ID<input required pattern="[a-f0-9]{32}" value={editor.source_node_id} onChange={e => setEditor({ ...editor, source_node_id: e.target.value })} /></label><label>Sending account<input required value={editor.source_owner} onChange={e => setEditor({ ...editor, source_owner: e.target.value })} /></label><p>Bind this credential to the sending server’s node ID and local coordination account. Enabled permissions allow same-project inbox delivery or heavy-worker reservations for this receiving account. They do not execute jobs; each worker must honor its current lease and fence.</p></>}
          </>}
          <p>Use the same credential in the outgoing server entry and the receiving server’s incoming grant. Each direction needs its own entry and grant. Copy a generated credential before saving; it cannot be viewed again afterward.</p>
          <label>{editor.existing ? 'Replacement credential' : 'Credential'}<input type={showCredential ? 'text' : 'password'} autoComplete="new-password" minLength={32} maxLength={4096} value={editor.credential} onChange={e => setEditor({ ...editor, credential: e.target.value })} placeholder={editor.existing ? 'Leave blank to keep the current credential' : 'At least 32 characters'} /></label>
          <div className="servers-actions">
            <label className="servers-check"><input type="checkbox" checked={showCredential} onChange={e => setShowCredential(e.target.checked)} />Show credential</label>
            <button type="button" onClick={() => { const bytes = crypto.getRandomValues(new Uint8Array(32)); setEditor({ ...editor, credential: Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('') }); setShowCredential(true); }}>Generate credential</button>
          </div>
          <details><summary>Use a service environment variable</summary>
            <label>Credential environment variable<input pattern="[A-Za-z_][A-Za-z0-9_]*" value={editor.token_env} onChange={e => setEditor({ ...editor, token_env: e.target.value })} /></label>
            <p>A replacement credential overrides this setting. Changes to the service environment require a service restart.</p>
          </details>
          <div className="servers-actions"><button type="submit" disabled={!directory}>Save {editor.kind === 'server' ? 'server' : 'grant'}</button><button type="button" onClick={() => { setEditor(null); setShowCredential(false); }}>Cancel</button></div>
        </fieldset>
      </form> : <>
        <div className="servers-actions"><button disabled={busy || !directory} onClick={() => edit('server')}>Add server</button><button disabled={busy} onClick={() => { void run(reload); }}>Refresh directory</button></div>
        {!directory && !error && <p>Loading servers…</p>}
        {directory?.servers.length === 0 && <p>No server peers configured. Add a server to browse its terminals.</p>}
        {directory?.servers.map(server => <section key={server.id} aria-label={`${server.name} configuration`}>
          <strong>{server.name}</strong><p>{server.url}</p>
          <p>{server.credential_ready ? 'Credential configured' : 'Credential unavailable — edit this server to supply a credential.'}</p>
          {server.coordination_owner && <p>Project coordination account: {server.coordination_owner} · Summaries{server.coordination_messages ? ' · Messages' : ''}{server.coordination_reservations ? ' · Worker reservations' : ''}</p>}
          {server.identity?.node_id ? <p>Verified node: {server.identity.node_id} · Account: {server.identity.owner}. Nearby address changes require the same verified identity.</p> : <p>Node identity has not been verified. Manual connection settings are retained.</p>}
          <div className="servers-actions">
            <button disabled={busy} onClick={() => edit('server', server)}>Edit {server.name}</button>
            {server.url.startsWith('https://') && <button disabled={busy || !server.credential_ready} onClick={() => { void run(async () => { await api.verifyServerIdentity(server.id); await reload(); setNotice(`${server.name}: node and receiving account identity verified.`); }); }}>Verify identity of {server.name}</button>}
            <button disabled={busy} onClick={() => { void run(async () => { const sessions = await api.getServerSessions(server.id); setNotice(`${server.name}: connection successful, ${sessions.length} sessions available.`); }); }}>Test connection to {server.name}</button>
            <button disabled={busy} onClick={() => remove('server', server.id)}>Remove {server.name}</button>
          </div>
        </section>)}
        <h3>Nearby servers</h3>
        <p>Network announcements are unverified. Confirm the server address and identity before sharing a credential. Discovery does not grant access.</p>
        {discoveryError && <p role="status">{discoveryError}</p>}
        {discovery?.error && <p role="status">{discovery.error}</p>}
        {discovery && !discovery.started && !discovery.error && <p>Network discovery is inactive for this listener.</p>}
        {discovery?.started && !discovery.policy.browse && <p>Network browsing is disabled by server policy.</p>}
        {discovery?.started && discovery.policy.browse && discovery.candidates.length === 0 && <p>No nearby servers discovered. You can still add a server manually.</p>}
        {discovery?.candidates.map(node => <section key={`${node.instance}/${node.address}`} aria-label={`${node.host} discovered server`}>
          <strong>{node.host}</strong><p>{node.address}:{node.port} · Unverified announcement</p>
          <p>Claimed node ID: {node.claimed_id}</p>
          <button disabled={busy || !directory} onClick={() => {
            const url = `${node.tls ? 'https' : 'http'}://${node.host}:${node.port}`;
            edit('server'); setEditor({ ...emptyEditor('server'), name: node.host, url });
          }}>Configure connection to {node.host}</button>
        </section>)}
        <h3>Incoming access</h3>
        {discovery?.policy.id && <p>This server’s node ID: <code>{discovery.policy.id}</code></p>}
        <p>Grant a trusted WebMux server access to one local account’s terminals. Terminals run as the WebMux service user.</p>
        <button disabled={busy || !directory} onClick={() => edit('grant')}>Add incoming grant</button>
        {directory?.grants.map(grant => <section key={grant.id} aria-label={`${grant.id} incoming grant`}>
          <strong>{grant.id}</strong><p>Owner: {grant.owner} · {grant.credential_ready ? 'Credential configured' : 'Credential unavailable'}</p>
          <p>{grant.coordination ? 'Project coordination summaries allowed over HTTPS' : 'Terminal access only'}</p>
          {grant.coordination_reservations && <p>Worker reservations allowed from node {grant.source_node_id} · Account: {grant.source_owner}</p>}
          {grant.coordination_messages && <p>Messages allowed from node {grant.source_node_id} · Account: {grant.source_owner}</p>}
          <div className="servers-actions"><button disabled={busy} onClick={() => edit('grant', grant)}>Edit grant {grant.id}</button><button disabled={busy} onClick={() => remove('grant', grant.id)}>Remove grant {grant.id}</button></div>
        </section>)}
      </>}
      {busy && <p role="status">Working…</p>}
    </div>
  </Dialog>;
}
