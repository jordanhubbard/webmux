import { useCallback, useEffect, useState } from 'react';
import { api } from '../utils/api';
import type { CoordinatedAgent } from './CoordinationDialog';

export function RigNavigator({ onOpen, onManage }: { onOpen: (target: string) => Promise<void>; onManage: () => void }) {
  const [agents, setAgents] = useState<CoordinatedAgent[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const refresh = useCallback(() => {
    void api.getCoordination().then(result => { setAgents(result.agents); setError(''); })
      .catch(reason => setError(String(reason)));
  }, []);
  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, 10000);
    window.addEventListener('webmux-sessions-changed', refresh);
    return () => { window.clearInterval(timer); window.removeEventListener('webmux-sessions-changed', refresh); };
  }, [refresh]);
  const projects = [...new Set(agents.map(agent => agent.project))].sort();
  return <details open style={{ background: '#141428', border: '1px solid #333366', borderRadius: 4, margin: '8px 8px 0', padding: '5px 8px', flexShrink: 0 }}>
    <summary style={{ cursor: 'pointer', color: '#c5b8ff' }}>Rig seats ({agents.length})</summary>
    {error && <p role="status">Rig status unavailable: {error}</p>}
    {!error && agents.length === 0 && <p>No managed seats yet. Use an empty grid cell to create one.</p>}
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, marginTop: 8 }}>
      {projects.map(project => <div key={project} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 5 }}>
        <strong>{project}</strong>
        {agents.filter(agent => agent.project === project).map(agent => <button key={agent.target} type="button" disabled={!!busy || !agent.connected}
          title={`${agent.name} · ${agent.host} · provider ${agent.sessionStatus || 'unknown'} · attention ${agent.activityState || 'unknown'} · ${agent.connected ? 'tmux available' : 'tmux unavailable'}`}
          onClick={() => { setBusy(agent.target); void onOpen(agent.target).catch(reason => setError(String(reason))).finally(() => setBusy('')); }}
          style={{ background: '#1a1a3a', color: '#ddd', border: '1px solid #555588', borderRadius: 4, padding: '3px 8px', cursor: 'pointer' }}>
          {agent.name} · {agent.host} · {agent.connected ? `tmux available · ${agent.activityState || 'activity unknown'}` : 'tmux unavailable'} · provider {agent.sessionStatus || 'unknown'}
        </button>)}
      </div>)}
      <button type="button" onClick={onManage}>Manage Rig seats</button>
    </div>
  </details>;
}
