import { useEffect, useState } from 'react';
import { Dialog } from './Dialog';
import './CoordinationDialog.css';
import { api } from '../utils/api';

export interface CoordinationSeat {
  id: string;
  title: string;
  sessionId?: string;
  minimized: boolean;
  status: string;
  external?: boolean;
}

export interface CoordinationProject {
  provider: string;
  name: string;
  seats: CoordinationSeat[];
}

export interface CoordinationStatus {
  enabledProviders: string[];
  defaultLaunchProvider: 'webmux';
  defaultView: 'all' | 'webmux';
  projects: CoordinationProject[];
  removedProviders?: string[];
  message?: string;
}

export function CoordinationDialog({ onClose, inline = false }: { onClose: () => void; inline?: boolean; position?: { row: number; col: number } }) {
  const [status, setStatus] = useState<CoordinationStatus | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    api.getCoordination(undefined, controller.signal).then(value => {
      if (!controller.signal.aborted) setStatus(value);
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Cannot load coordination');
    });
    return () => controller.abort();
  }, []);
  const reopen = async (id: string) => {
    setBusy(true);
    setError('');
    try {
      await api.attachCoordinatedSession(id);
      window.dispatchEvent(new CustomEvent('webmux-sessions-changed', { detail: { sessionId: id } }));
      if (!inline) onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Cannot reopen the terminal');
    } finally {
      setBusy(false);
    }
  };
  const body = (
    <div role="region" aria-label="Coordination">
      <h2>Coordination</h2>
      <p>{status?.message || 'Agent Deck and OpenRig are no longer launched by WebMux. Existing tmux sessions were left running. Attach one from a WebMux terminal with tmux attach. WebMux does not stop them.'}</p>
      {error && <p role="alert">{error}</p>}
      {!status && !error && <p role="status">Loading terminals…</p>}
      {status?.projects.map(project => (
        <section key={project.name}>
          <h3>{project.name}</h3>
          {project.seats.length === 0 && <p>No terminals in this project.</p>}
          <ul>
            {project.seats.map(seat => (
              <li key={seat.id}>
                {seat.title || seat.id} · {seat.status}{seat.external ? ' · external tmux' : ''}{seat.minimized ? ' · minimized' : ''}
                {' '}
                <button type="button" disabled={busy} onClick={() => void reopen(seat.sessionId || seat.id)}>Reopen</button>
              </li>
            ))}
          </ul>
        </section>
      ))}
      {status && status.projects.length === 0 && <p>No WebMux terminals are open.</p>}
    </div>
  );
  if (inline) return body;
  return <Dialog title="Coordination" onClose={onClose}>{body}</Dialog>;
}
