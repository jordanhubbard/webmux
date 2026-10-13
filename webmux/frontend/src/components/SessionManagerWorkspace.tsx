import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../utils/api';
import './SessionManagerWorkspace.css';

interface ManagedSession {
  id: string; title: string; cwd: string; state: string; parentId?: string;
  summary: string; summarySource: string; observedAt: number;
  attachments: { client: string }[];
  capabilities: { permissions: boolean };
}
interface Inventory { sessions: ManagedSession[]; nextCursor: string | null; coverage: { complete: boolean; message: string } }
interface Pending { id: string | number; method: string; terminalOnly?: boolean; params: { questions?: { id: string; question: string }[]; [key: string]: unknown } }
interface Detail { session: ManagedSession; items: { id: string; type: string; text: string }[]; requests: Pending[]; turnId: string | null; observedAt: number }
interface Permissions { revision: string; effective: { approval: unknown; sandbox: unknown; profile: unknown }; appliesTo: string }
type Policy = { access: string; approval: string };
const initialPolicy = { access: 'workspace-write', approval: 'on-request' };

export function SessionManagerWorkspace() {
  const [clientId] = useState(() => crypto.randomUUID());
  const [inventory, setInventory] = useState<Inventory | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [permissions, setPermissions] = useState<Permissions | null>(null);
  const [defaults, setDefaults] = useState<{ revision: string; policy: Policy | null } | null>(null);
  const [policy, setPolicy] = useState<Policy>(initialPolicy);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [cwd, setCwd] = useState('');
  const [query, setQuery] = useState('');
  const [history, setHistory] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [inspected, setInspected] = useState(false);
  const [presetChosen, setPresetChosen] = useState(false);
  const uncertainTarget = useRef<string | null | undefined>(undefined);
  const serviceIdentity = useRef<string | null>(null);
  const serviceEpoch = useRef(0);
  const epoch = useRef(0), selectedRef = useRef(selected), loading = useRef(false), alive = useRef(true), writing = useRef(false);
  selectedRef.current = selected;
  const call = useCallback(async <T,>(operation: string, params: Record<string, unknown> = {}) => {
    const requestEpoch = serviceEpoch.current;
    const response = await api.sessionManager<T>(operation, params, clientId);
    if (response.version !== 1) throw new Error('Unsupported session-manager version');
    if (requestEpoch !== serviceEpoch.current) throw new Error('A newer manager connection superseded this response. Refresh before another action.');
    const identity = `${response.hostId}/${response.providerId}/${response.generation}`;
    if (serviceIdentity.current !== null && serviceIdentity.current !== identity) {
      serviceIdentity.current = identity; ++serviceEpoch.current;
      ++epoch.current; selectedRef.current = null; setSelected(null); setDetail(null); setPermissions(null); setDefaults(null); setInventory(null);
      setUncertain(true); setInspected(false); uncertainTarget.current = null;
      throw new Error('The host runtime changed. Refresh the inventory and inspect before another action.');
    }
    serviceIdentity.current = identity;
    return response.result;
  }, [clientId]);
  const refresh = useCallback(async (more = false) => {
    if (loading.current || writing.current) return;
    loading.current = true;
    const revision = epoch.current, id = selectedRef.current;
    try {
      const result = await call<Inventory>('list', more && inventory?.nextCursor ? { cursor: inventory.nextCursor } : {});
      if (!alive.current || revision !== epoch.current) return;
      setInventory(previous => more && previous ? { ...result, sessions: [...new Map([...previous.sessions, ...result.sessions].map(s => [s.id, s])).values()] } : result);
      if (id) {
        const next = await call<Detail>('read', { sessionId: id });
        if (alive.current && revision === epoch.current && selectedRef.current === id) setDetail(next);
      }
      if (alive.current && revision === epoch.current) {
        setError(null);
        if (uncertainTarget.current !== undefined && (uncertainTarget.current === null || uncertainTarget.current === id)) setInspected(true);
      }
    } catch (e) { if (alive.current && revision === epoch.current) { setError((e as Error).message); setDetail(null); } }
    finally { loading.current = false; }
  }, [call, inventory?.nextCursor]);
  const refreshRef = useRef(refresh); refreshRef.current = refresh;
  useEffect(() => {
    alive.current = true;
    void refreshRef.current();
    const timer = window.setInterval(() => { void refreshRef.current(); }, 5000);
    return () => { alive.current = false; ++epoch.current; window.clearInterval(timer); if (selectedRef.current) void call('detach', { sessionId: selectedRef.current }).catch(() => {}); };
  }, [call]);
  const act = async (work: () => Promise<void>) => {
    if (writing.current) return;
    writing.current = true; setBusy(true); setError(null); ++epoch.current;
    try { await work(); }
    catch (e) { if (alive.current) { setError(`${(e as Error).message} Nothing was automatically retried. Refresh and inspect before another change.`); setUncertain(true); setInspected(false); uncertainTarget.current = selectedRef.current; setPermissions(null); } }
    finally { writing.current = false; if (alive.current) setBusy(false); }
  };
  const attach = (id: string) => act(async () => {
    const old = selectedRef.current;
    if (old) await call('detach', { sessionId: old });
    await call('attach', { sessionId: id });
    if (!alive.current) { await call('detach', { sessionId: id }); return; }
    selectedRef.current = id; setSelected(id); setDetail(null); setPermissions(null);
    const next = await call<Detail>('read', { sessionId: id });
    if (alive.current && selectedRef.current === id) setDetail(next);
  });
  const loadPermissions = () => act(async () => {
    if (!selected) return;
    const result = await call<Permissions>('permissions.read', { sessionId: selected });
    if (alive.current && selectedRef.current === selected) { setPermissions(result); setPresetChosen(false); }
  });
  const policyFields = <>
    <label>Replacement access preset <select value={policy.access} onChange={e => { setPresetChosen(true); setPolicy({ ...policy, access: e.target.value }); }}>
      <option value="read-only">Read only</option><option value="workspace-write">Workspace write · restricted network</option><option value="danger-full-access">Full access</option>
    </select></label>
    <label>Replacement approval preset <select value={policy.approval} onChange={e => { setPresetChosen(true); setPolicy({ ...policy, approval: e.target.value }); }}>
      <option value="on-request">When requested</option><option value="untrusted">Untrusted commands</option><option value="never">Never ask</option>
    </select></label>
  </>;
  const sessions = inventory?.sessions.filter(s => (history || s.state !== 'notLoaded') && `${s.title} ${s.cwd} ${s.id}`.toLowerCase().includes(query.toLowerCase())) ?? [];
  return <div className="session-manager">
    <header><h2>Host sessions</h2><button disabled={busy} onClick={() => { void refresh(); }}>Refresh</button></header>
    <p>{inventory?.coverage.message ?? 'Connect to the configured local session manager.'}</p>
    {inventory && !inventory.coverage.complete && <p role="status">Some runtime sessions could not be checked.</p>}
    {error && <p role="alert">{error}</p>}
    {uncertain && <p role="status">A previous operation needs review. Refresh its session before making another change. <button disabled={!inspected || busy} onClick={() => { setUncertain(false); setInspected(false); uncertainTarget.current = undefined; }}>I have reviewed the result; allow another action</button></p>}
    <div className="session-manager-columns">
      <aside>
        <input aria-label="Search sessions" placeholder="Search title, folder or ID" value={query} onChange={e => setQuery(e.target.value)} />
        <label><input type="checkbox" checked={history} onChange={e => setHistory(e.target.checked)} /> Include saved history</label>
        {sessions.map(s => <button className="session-manager-card" aria-pressed={selected === s.id} key={s.id} disabled={busy} onClick={() => void attach(s.id)}>
          <strong>{s.title}</strong><span>{s.cwd}</span><span>{s.state} · {s.id.slice(0, 8)}{s.parentId ? ' · child session' : ''}</span>
          <span>{s.summary}</span><small>{s.summarySource === 'recent-items' ? 'Recent progress' : 'Saved preview'} · checked {new Date(s.observedAt).toLocaleTimeString()}</small>
          <small>{s.attachments.map(a => a.client).join(', ') || 'No registered client attachment'}</small>
        </button>)}
        {inventory?.nextCursor && <button disabled={busy} onClick={() => void refresh(true)}>Load more history</button>}
      </aside>
      <main>
        {selected && <button disabled={busy} onClick={() => void act(async () => { await call('detach', { sessionId: selected }); selectedRef.current = null; setSelected(null); setDetail(null); setPermissions(null); })}>Disconnect · leave work running</button>}
        {detail ? <>
          <h3>{detail.session.title}</h3><p>{detail.session.state} · checked {new Date(detail.observedAt).toLocaleTimeString()}</p>
          <div className="session-manager-transcript">{[...detail.items].reverse().map((item, i) => <div key={item.id ?? i}><small>{item.type}</small><pre>{item.text}</pre></div>)}</div>
          {detail.requests.map(request => <PendingControl key={String(request.id)} request={request} disabled={busy || uncertain} answer={result => void act(async () => { await call('respond', { sessionId: selected, id: request.id, result }); setDetail(null); })} />)}
          <textarea aria-label="Message to session" value={drafts[selected!] ?? ''} onChange={e => setDrafts({ ...drafts, [selected!]: e.target.value })} />
          <button disabled={busy || uncertain || !(drafts[selected!] ?? '').trim()} onClick={() => void act(async () => {
            const text = drafts[selected!]; await call('send', { sessionId: selected, text, turnId: detail.turnId });
            setDrafts(current => current[selected!] === text ? { ...current, [selected!]: '' } : current); setDetail(null);
          })}>Send</button>
          {detail.turnId && <button disabled={busy || uncertain} onClick={() => void act(async () => { await call('stop', { sessionId: selected, turnId: detail.turnId }); setDetail(null); })}>Stop current turn</button>}
          {detail.session.capabilities.permissions && <button disabled={busy} onClick={() => void loadPermissions()}>Inspect permissions</button>}
        </> : <p>{selected ? 'Refresh to read this conversation.' : 'Connect to a session to view recent work and send a message.'}</p>}
        {permissions && <section><h3>Permissions for subsequent turns</h3><pre>{JSON.stringify(permissions.effective, null, 2)}</pre>
          <p>Changes do not approve a waiting command or restart the current turn. “Never ask” does not override restricted access.</p>
          <p>Selecting a preset replaces any custom roots, network rules or named profile shown above.</p>
          {policyFields}<button disabled={busy || uncertain || !presetChosen} onClick={() => void act(async () => {
            const result = await call<Permissions>('permissions.update', { sessionId: selected, revision: permissions.revision, policy });
            if (alive.current) { setPermissions(result); setPresetChosen(false); }
          })}>Apply to this session</button>
        </section>}
        <details><summary>Create session and defaults</summary>
          {policyFields}<label>Workspace <input value={cwd} onChange={e => setCwd(e.target.value)} placeholder="Absolute host folder" /></label>
          <button disabled={busy || uncertain || !cwd.startsWith('/')} onClick={() => void act(async () => {
            const result = await call<{ session: ManagedSession }>('create', { cwd, policy });
            setInventory(current => current ? { ...current, sessions: [result.session, ...current.sessions] } : current);
          })}>Create with these permissions</button>
          <button disabled={busy} onClick={() => void act(async () => {
            const result = await call<{ revision: string; policy: Policy | null }>('defaults.read'); setDefaults(result); if (result.policy) setPolicy(result.policy);
          })}>Read creation default</button>
          {defaults && <><p>Default: {defaults.policy ? `${defaults.policy.access}, ${defaults.policy.approval}` : 'No default selected'}</p><button disabled={busy || uncertain} onClick={() => void act(async () => {
            setDefaults(await call('defaults.update', { revision: defaults.revision, policy }));
          })}>Save creation default</button></>}
        </details>
      </main>
    </div>
  </div>;
}

function PendingControl({ request, disabled, answer }: { request: Pending; disabled: boolean; answer: (result: unknown) => void }) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const offered = Array.isArray(request.params.availableDecisions) ? request.params.availableDecisions : ['accept', 'decline', 'cancel'];
  if (request.terminalOnly) return <p>Request requires the owning runtime’s client.</p>;
  if (request.method === 'item/tool/requestUserInput') return <fieldset disabled={disabled}>
    {request.params.questions?.map(q => <label key={q.id}>{q.question}<input value={answers[q.id] ?? ''} onChange={e => setAnswers({ ...answers, [q.id]: e.target.value })} /></label>)}
    <button disabled={disabled || !request.params.questions?.every(q => answers[q.id]?.trim())} onClick={() => answer({ answers: Object.fromEntries(Object.entries(answers).map(([id, text]) => [id, { answers: [text] }])) })}>Answer</button>
  </fieldset>;
  if (['item/commandExecution/requestApproval', 'item/fileChange/requestApproval'].includes(request.method)) return <fieldset disabled={disabled}><legend>Approval requested</legend>
    <pre>{JSON.stringify(request.params, null, 2)}</pre>
    {offered.includes('accept') && <button onClick={() => answer({ decision: 'accept' })}>Approve once</button>}{offered.includes('decline') && <button onClick={() => answer({ decision: 'decline' })}>Decline</button>}{offered.includes('cancel') && <button onClick={() => answer({ decision: 'cancel' })}>Cancel request</button>}
  </fieldset>;
  return <p>Permission request: review in the terminal or another capable client.</p>;
}
