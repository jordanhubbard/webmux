import { useEffect, useState } from 'react';
import { api } from '../utils/api';
import { executionHostLabel } from '../utils/sessionIdentity';
import type { CoordinationProject, CoordinationSeat } from './CoordinationDialog';

const lifecycleLabel = (seat: CoordinationSeat) => seat.observationState && seat.observationState !== 'current' ? `last observed ${seat.state}; ${seat.observationState}` : seat.state;

export function RigNavigator({ onOpen, onManage, active = true, disabled = false, openRequest = 0 }: {
  onOpen: (provider: string, target: string, name?: string) => Promise<void>; onManage: () => void;
  active?: boolean; disabled?: boolean; openRequest?: number;
}) {
  const [projects, setProjects] = useState<CoordinationProject[]>([]);
  const [seats, setSeats] = useState<CoordinationSeat[]>([]);
  const [enabled, setEnabled] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  useEffect(() => {
    if (openRequest > 0) { setLoading(true); setExpanded(true); }
  }, [openRequest]);
  useEffect(() => {
    if (!active || !expanded) return;
    let current = true;
    let request: AbortController | undefined;
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      if (request) return;
      clearTimeout(timer);
      const controller = new AbortController();
      request = controller;
      try {
        const result = await api.getCoordination(undefined, controller.signal);
        if (!current) return;
        const external = (provider: string) => provider === 'rig' || provider === 'agent-deck';
        setEnabled((result.enabledProviders || []).some(external));
        setProjects((result.projects || []).filter(value => external(value.provider)));
        setSeats((result.seats || []).filter(value => external(value.provider)));
        setError(''); setLoading(false);
      } catch (reason) {
        if (!current) return;
        setProjects([]); setSeats([]); setError(String(reason)); setLoading(false);
      } finally {
        request = undefined;
        if (current) timer = setTimeout(() => { void refresh(); }, 10000);
      }
    };
    const changed = () => { void refresh(); };
    void refresh();
    window.addEventListener('webmux-sessions-changed', changed);
    return () => { current = false; setLoading(true); request?.abort(); clearTimeout(timer); window.removeEventListener('webmux-sessions-changed', changed); };
  }, [active, expanded, openRequest]);
  return <details open={expanded} onToggle={event => { const open = event.currentTarget.open; if (open !== expanded) { setLoading(true); setExpanded(open); } }} style={{ background: '#141428', border: '1px solid #333366', borderRadius: 4, margin: '8px 8px 0', padding: '5px 8px', flexShrink: 0 }}>
    <summary style={{ cursor: 'pointer', color: '#c5b8ff' }}>Coordinated sessions{!loading && ` (${seats.length})`}</summary>
    {error && <p role="status">Coordination status unavailable: {error}</p>}
    {loading && expanded && <p role="status">Checking coordinated sessions…</p>}
    {!loading && !error && seats.length === 0 && <p>{enabled ? 'No coordinator-managed sessions yet. Use an empty grid cell to create one.' : 'Enable Agent Deck or Rig in Coordination to list seats.'}</p>}
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, marginTop: 8 }}>
      {projects.map(project => <div key={`${project.provider}:${project.id}`} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 5 }}>
        <strong>{project.name}</strong>
        {seats.filter(seat => seat.projectId === project.id && seat.provider === project.provider).map(seat => <button key={`${seat.provider}:${seat.id}`} type="button" disabled={!active || loading || disabled || !!busy || !seat.openable}
          title={`${seat.name} · ${executionHostLabel(seat.host)} · ${seat.runtime} · ${lifecycleLabel(seat)} · ${seat.transportState ? `attachment ${seat.transportState} · ` : ''} ${seat.workspace || 'workspace unknown'}`}
          onClick={() => { const key = `${seat.provider}:${seat.id}`; setBusy(key); void onOpen(seat.provider, seat.id, seat.name).catch(reason => setError(String(reason))).finally(() => setBusy('')); }}
          style={{ background: '#1a1a3a', color: '#ddd', border: '1px solid #555588', borderRadius: 4, padding: '3px 8px', cursor: 'pointer' }}>
          {seat.name} · {executionHostLabel(seat.host)} · {lifecycleLabel(seat)} · {seat.provider === 'agent-deck' ? 'Agent Deck' : 'Rig'}
        </button>)}
      </div>)}
      <button type="button" onClick={onManage}>Manage coordination</button>
    </div>
  </details>;
}
