import { useState } from 'react';
import { api, type CredentialConsumer, type CredentialHubInfo, type ServerDirectory, type SharedCredential } from '../utils/api';

const providerLabel = (consumer: CredentialConsumer) => consumer === 'ai.openai' ? 'AI assistant · OpenAI' : 'AI assistant · NVIDIA';
function ProviderSelect({ label, value, onChange, disabled = false }: { label: string; value: CredentialConsumer; onChange: (value: CredentialConsumer) => void; disabled?: boolean }) {
  return <select aria-label={label} disabled={disabled} value={value} onChange={event => onChange(event.target.value as CredentialConsumer)}><option value="ai.openai">AI assistant · OpenAI</option><option value="ai.nvidia">AI assistant · NVIDIA</option></select>;
}

export function CredentialHubPanel({ directory, currentUser, localNode }: { directory: ServerDirectory; currentUser: string; localNode?: string }) {
  const [open, setOpen] = useState(false);
  const [info, setInfo] = useState<CredentialHubInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [message, setMessage] = useState('');
  const [reply, setReply] = useState('');
  const [editing, setEditing] = useState<SharedCredential | null>(null);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [consumer, setConsumer] = useState<CredentialConsumer>('ai.openai');
  const [enabled, setEnabled] = useState(true);
  const [grantSecret, setGrantSecret] = useState('');
  const [enrollment, setEnrollment] = useState('');
  const [peer, setPeer] = useState('');
  const [remoteConsumer, setRemoteConsumer] = useState<CredentialConsumer>('ai.openai');
  const [available, setAvailable] = useState<SharedCredential[] | null>(null);
  const [selection, setSelection] = useState('');
  const run = async (action: () => Promise<void>) => {
    setBusy(true); setError(''); setNotice('');
    try { await action(); } catch (cause) { setError(cause instanceof Error ? cause.message : 'Credential operation failed'); } finally { setBusy(false); }
  };
  const reload = async () => setInfo(await api.getCredentialHub());
  const reset = () => { setEditing(null); setName(''); setValue(''); setConsumer('ai.openai'); setEnabled(true); };
  const incoming = directory.grants.filter(grant => grant.credential_sharing && grant.owner === currentUser);
  const peers = directory.servers.filter(server => server.identity?.node_id && server.url.startsWith('https://') && !server.allow_http);
  const secrets = info?.vault.secrets ?? [];
  const selectedSecret = secrets.find(secret => secret.id === grantSecret);
  return <section aria-label="Credential sharing">
    <h3>Credential sharing</h3>
    {localNode && <p aria-label="This server credential identity">This server node: <code>{localNode}</code> · Account: {currentUser}</p>}
    <p>Use a linked WebMux server as an API-key hub. Sharing is independent of MAC and never changes native provider login profiles.</p>
    {!open ? <button type="button" onClick={() => { setOpen(true); void run(reload); }}>Manage shared credentials</button> : <>
      {error && <p role="alert" aria-label="Credential operation error">{error}</p>}
      {notice && <p role="status" aria-label="Credential operation status">{notice}</p>}
      <fieldset disabled={busy}>
        <button type="button" onClick={() => { void run(reload); }}>Refresh credential status</button>
        {!info && !error && <p>Loading credential status…</p>}
        {info && <>
          <h4>Store credentials on this server</h4>
          <p>Keys are encrypted with a separate owner-only key file. The owning operating-system account can access them. Back up both vault files privately; losing the key makes stored values unavailable.</p>
          {!info.storage_supported && <p>This platform cannot provide the supported protected hub storage. Select a supported linked hub below.</p>}
          {!info.vault.enabled ? <button type="button" disabled={!info.storage_supported} onClick={() => { void run(async () => { await api.enableCredentialHub(); await reload(); setNotice('Encrypted storage enabled for this account.'); }); }}>Enable encrypted credential storage</button> : <>
            <form aria-label="Store shared credential" onSubmit={event => {
              event.preventDefault();
              void run(async () => {
                await api.putSharedCredential(editing?.id ?? null, { name, value, consumer, enabled });
                reset(); await reload(); setNotice('Credential saved. Its value is never returned to this browser.');
              });
            }}>
              <label>Credential name<input required maxLength={80} value={name} onChange={event => setName(event.target.value)} /></label>
              <label>Credential consumer<ProviderSelect label="Credential consumer" value={consumer} disabled={!!editing} onChange={setConsumer} /></label>
              <label>{editing ? 'Replacement API key' : 'API key'}<input type="password" autoComplete="new-password" required={!editing} maxLength={4096} value={value} onChange={event => setValue(event.target.value)} placeholder={editing ? 'Leave blank to keep the current value' : undefined} /></label>
              <label className="servers-check"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />Credential enabled</label>
              <button type="submit">{editing ? 'Save credential changes' : 'Store credential'}</button>
              {editing && <button type="button" onClick={reset}>Cancel credential changes</button>}
            </form>
            {secrets.length === 0 && <p>No shared credentials stored.</p>}
            {secrets.map(secret => <section key={secret.id} aria-label={`${secret.name} shared credential`}>
              <strong>{secret.name}</strong><p>{providerLabel(secret.consumer)} · Version {secret.version} · {secret.enabled ? 'Enabled' : 'Disabled'}</p>
              <p>Credential ID: <code>{secret.id}</code></p>
              <button type="button" onClick={() => { setEditing(secret); setName(secret.name); setConsumer(secret.consumer); setEnabled(secret.enabled); setValue(''); }}>Edit {secret.name}</button>
              <button type="button" onClick={() => { if (window.confirm(`Delete ${secret.name} and all its grants?`)) void run(async () => { await api.deleteSharedCredential(secret.id); if (editing?.id === secret.id) reset(); await reload(); }); }}>Delete {secret.name}</button>
            </section>)}
            <h4>Grant access to a linked account</h4>
            <p>Enable credential hub requests on an incoming grant first. Bind it to the receiving server’s node ID and account, with its own unique link credential. Editing or recreating that enrollment requires granting named credentials again.</p>
            <form aria-label="Grant shared credential" onSubmit={event => { event.preventDefault(); if (!selectedSecret) return; void run(async () => { await api.grantSharedCredential({ secret_id: selectedSecret.id, enrollment_id: enrollment, consumer: selectedSecret.consumer }); await reload(); setNotice('Credential access granted to the selected node and account.'); }); }}>
              <label>Credential to share<select aria-label="Credential to share" required value={grantSecret} onChange={event => setGrantSecret(event.target.value)}><option value="">Choose credential</option>{secrets.filter(secret => secret.enabled).map(secret => <option key={secret.id} value={secret.id}>{secret.name}</option>)}</select></label>
              <label>Receiving enrollment<select aria-label="Receiving enrollment" required value={enrollment} onChange={event => setEnrollment(event.target.value)}><option value="">Choose bound incoming grant</option>{incoming.map(grant => <option key={grant.id} value={grant.id}>{grant.id} · {grant.source_owner} · {grant.source_node_id}</option>)}</select></label>
              <button type="submit" disabled={!selectedSecret || !enrollment}>Grant credential access</button>
            </form>
            {info.vault.grants.map(grant => <section key={grant.id} aria-label="Shared credential grant">
              <p>{secrets.find(secret => secret.id === grant.secret_id)?.name ?? grant.secret_id} → {grant.enrollment_id} · {grant.source_owner} · {grant.source_node} · {grant.active ? 'Active' : 'No current access; remove and grant again after checking the enrollment'}</p>
              <button type="button" onClick={() => { void run(async () => { await api.revokeSharedCredential(grant.id); await reload(); setNotice('Grant revoked. Existing provider keys may also need provider-side rotation.'); }); }}>Revoke credential grant {grant.enrollment_id}</button>
            </section>)}
          </>}
          <h4>Use a linked credential hub</h4>
          <p>The selected key is fetched privately for each AI request. Hub outages, changed identities and revocations refuse new requests without using another provider credential.</p>
          {info.binding && <><p role="status" aria-label="Selected credential hub">Selected hub: {directory.servers.find(server => server.id === info.binding?.peer)?.name ?? info.binding.peer} · {providerLabel(info.binding.consumer)} · {info.binding.secret_id}</p><button type="button" onClick={() => { void run(async () => { await api.clearCredentialHub(); await reload(); setNotice('Hub selection removed. The existing local AI configuration applies again.'); }); }}>Use local AI configuration</button></>}
          <label>Credential hub server<select aria-label="Credential hub server" value={peer} onChange={event => { setPeer(event.target.value); setAvailable(null); setSelection(''); }}><option value="">Choose verified HTTPS server</option>{peers.map(server => <option key={server.id} value={server.id}>{server.name}</option>)}</select></label>
          <label>Hub credential consumer<ProviderSelect label="Hub credential consumer" value={remoteConsumer} onChange={next => { setRemoteConsumer(next); setAvailable(null); setSelection(''); }} /></label>
          <button type="button" disabled={!peer} onClick={() => { void run(async () => { const result = await api.getAvailableCredentials(peer, remoteConsumer); setAvailable(result); setSelection(''); }); }}>Load granted credential names</button>
          {available && <><label>Granted credential<select aria-label="Granted credential" value={selection} onChange={event => setSelection(event.target.value)}><option value="">Choose named credential</option>{available.map(secret => <option key={secret.id} value={secret.id}>{secret.name} · Version {secret.version}</option>)}</select></label>{available.length === 0 && <p>This hub grants no matching credentials to this server and account.</p>}<button type="button" disabled={!selection} onClick={() => { void run(async () => { await api.selectCredentialHub({ peer, secret_id: selection, consumer: remoteConsumer }); await reload(); setNotice('Credential hub selected. The AI assistant below now uses its granted key.'); }); }}>Use selected credential hub</button></>}
          {info.binding && <form aria-label="AI assistant with shared credential" onSubmit={event => { event.preventDefault(); void run(async () => { setReply(''); const result = await api.credentialAIChat(message); setReply(result.reply); }); }}>
            <h4>AI assistant with shared credential</h4>
            <label>AI request<textarea required maxLength={4000} value={message} onChange={event => setMessage(event.target.value)} /></label>
            <button type="submit">Ask AI with shared credential</button>
            {reply && <p role="status" aria-label="Shared credential AI reply">{reply}</p>}
          </form>}
          <details><summary>Credential access audit</summary><p>Latest {info.vault.audit.length} retained events. Values and retrieval handles are excluded.</p><ul>{info.vault.audit.slice(-30).reverse().map((entry, index) => <li key={`${entry.at}/${index}`}>{entry.at} · {entry.operation} · {entry.allowed ? 'Allowed' : 'Refused'}{entry.enrollment_id ? ` · ${entry.enrollment_id}` : ''}</li>)}</ul></details>
        </>}
      </fieldset>
      {busy && <p role="status">Updating credential configuration…</p>}
    </>}
  </section>;
}
