import { useEffect, useState } from 'react';
import { api, type ManagedServer, type ServerGrant, type ServerDirectory } from '../utils/api';
import { Dialog } from './Dialog';
import './ServersDialog.css';

type Editor = { kind: 'server' | 'grant'; existing: boolean; id: string; name: string; url: string; owner: string; allow_http: boolean; token_env: string; credential: string };
const emptyEditor = (kind: Editor['kind']): Editor => ({ kind, existing: false, id: '', name: '', url: '', owner: '', allow_http: false, token_env: '', credential: '' });

function serverIdentity(url: string, directory: ServerDirectory) {
  const parsed = new URL(url);
  const base = parsed.host
    .toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 56) || 'server';
  const used = new Set(['local', 'all', ...directory.servers.map(server => server.id), ...directory.grants.map(grant => grant.id)]);
  let id = base;
  for (let suffix = 2; used.has(id); suffix++) id = `${base}-${suffix}`;
  return { id, name: parsed.host };
}

export function ServersDialog({ onClose, onChanged, currentUser, initiallyAddServer = false }: { initiallyAddServer?: boolean; onClose: () => void; onChanged: () => Promise<void>; currentUser: string | null }) {
  const [directory, setDirectory] = useState<ServerDirectory | null>(null);
  const [editor, setEditor] = useState<Editor | null>(() => initiallyAddServer ? emptyEditor('server') : null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [showCredential, setShowCredential] = useState(false);
  useEffect(() => {
    let canceled = false;
    void api.getServerDirectory().then(value => { if (!canceled) setDirectory(value); }).catch(error => { if (!canceled) setError(String(error)); });
    return () => { canceled = true; };
  }, []);
  const run = async (action: () => Promise<void>) => {
    setBusy(true); setError(''); setNotice('');
    try { await action(); } catch (error) { setError(String(error)); } finally { setBusy(false); }
  };
  const reload = async () => { setDirectory(await api.getServerDirectory()); await onChanged(); };
  const edit = (kind: Editor['kind'], entry?: ManagedServer | ServerGrant) => {
    setError(''); setNotice(''); setShowCredential(false);
    setEditor({ ...emptyEditor(kind), owner: currentUser || 'anonymous', ...entry, kind, existing: !!entry, credential: '' });
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
            const identity = input.existing ? input : serverIdentity(input.url, directory);
            await api.saveServer(identity.id, { name: input.name.trim() || identity.name, url: input.url, allow_http: input.allow_http, token_env: input.token_env, credential: input.credential });
          } else await api.saveServerGrant(input.id, { owner: input.owner, token_env: input.token_env, credential: input.credential });
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
          </> : <>
            <label>Grant name<input data-dialog-autofocus required pattern="[A-Za-z0-9][A-Za-z0-9_-]{0,63}" maxLength={64} disabled={editor.existing} value={editor.id} onChange={e => setEditor({ ...editor, id: e.target.value })} /></label>
            <label>Session owner<input required value={editor.owner} onChange={e => setEditor({ ...editor, owner: e.target.value })} /></label>
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
          <div className="servers-actions">
            <button disabled={busy} onClick={() => edit('server', server)}>Edit {server.name}</button>
            <button disabled={busy} onClick={() => { void run(async () => { const sessions = await api.getServerSessions(server.id); setNotice(`${server.name}: connection successful, ${sessions.length} sessions available.`); }); }}>Test connection to {server.name}</button>
            <button disabled={busy} onClick={() => remove('server', server.id)}>Remove {server.name}</button>
          </div>
        </section>)}
        <h3>Incoming access</h3>
        <p>Grant a trusted WebMux server access to one local account’s terminals. Terminals run as the WebMux service user.</p>
        <button disabled={busy || !directory} onClick={() => edit('grant')}>Add incoming grant</button>
        {directory?.grants.map(grant => <section key={grant.id} aria-label={`${grant.id} incoming grant`}>
          <strong>{grant.id}</strong><p>Owner: {grant.owner} · {grant.credential_ready ? 'Credential configured' : 'Credential unavailable'}</p>
          <div className="servers-actions"><button disabled={busy} onClick={() => edit('grant', grant)}>Edit grant {grant.id}</button><button disabled={busy} onClick={() => remove('grant', grant.id)}>Remove grant {grant.id}</button></div>
        </section>)}
      </>}
      {busy && <p role="status">Working…</p>}
    </div>
  </Dialog>;
}
