import { useCallback, useEffect, useState } from 'react';
import { api } from '../utils/api';
import type { CoordinationProject, CoordinationSeat } from './CoordinationDialog';

export function RigNavigator({ onOpen, onManage }: { onOpen: (provider: string, target: string) => Promise<void>; onManage: () => void }) {
  const [projects, setProjects] = useState<CoordinationProject[]>([]);
  const [seats, setSeats] = useState<CoordinationSeat[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const refresh = useCallback(() => {
    void api.getCoordination().then(result => { setProjects(result.projects || []); setSeats(result.seats || []); setError(''); })
      .catch(reason => setError(String(reason)));
  }, []);
  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, 10000);
    window.addEventListener('webmux-sessions-changed', refresh);
    return () => { window.clearInterval(timer); window.removeEventListener('webmux-sessions-changed', refresh); };
  }, [refresh]);
  return <details open style={{ background: '#141428', border: '1px solid #333366', borderRadius: 4, margin: '8px 8px 0', padding: '5px 8px', flexShrink: 0 }}>
    <summary style={{ cursor: 'pointer', color: '#c5b8ff' }}>Coordinated sessions ({seats.length})</summary>
    {error && <p role="status">Coordination status unavailable: {error}</p>}
    {!error && seats.length === 0 && <p>No coordinator-managed sessions yet. Use an empty grid cell to create one.</p>}
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, marginTop: 8 }}>
      {projects.map(project => <div key={`${project.provider}:${project.id}`} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 5 }}>
        <strong>{project.name}</strong>
        {seats.filter(seat => seat.projectId === project.id && seat.provider === project.provider).map(seat => <button key={`${seat.provider}:${seat.id}`} type="button" disabled={!!busy || !seat.openable}
          title={`${seat.name} · ${seat.host} · ${seat.runtime} · ${seat.state} · ${seat.workspace || 'workspace unknown'}`}
          onClick={() => { const key = `${seat.provider}:${seat.id}`; setBusy(key); void onOpen(seat.provider, seat.id).catch(reason => setError(String(reason))).finally(() => setBusy('')); }}
          style={{ background: '#1a1a3a', color: '#ddd', border: '1px solid #555588', borderRadius: 4, padding: '3px 8px', cursor: 'pointer' }}>
          {seat.name} · {seat.host} · {seat.state} · {seat.provider === 'agent-deck' ? 'Agent Deck' : 'Rig'}
        </button>)}
      </div>)}
      <button type="button" onClick={onManage}>Manage coordination</button>
    </div>
  </details>;
}
