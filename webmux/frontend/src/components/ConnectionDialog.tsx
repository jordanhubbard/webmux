import { useState, useEffect, FormEvent } from 'react';
import { api } from '../utils/api';
import type { HostEntry, CreateSessionRequest } from '../types';

interface ConnectionDialogProps {
  onConnect: (req: CreateSessionRequest) => void;
  onClose: () => void;
  suggestedRow?: number;
  suggestedCol?: number;
}

export function ConnectionDialog({ onConnect, onClose, suggestedRow, suggestedCol }: ConnectionDialogProps) {
  const [hosts, setHosts] = useState<HostEntry[]>([]);
  const [mode, setMode] = useState<'host' | 'adhoc'>('host');
  const [selectedHostId, setSelectedHostId] = useState('');
  const [adhocHostname, setAdhocHostname] = useState('');
  const [adhocPort, setAdhocPort] = useState(22);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState<'password' | 'key'>('password');
  const [transport, setTransport] = useState<'ssh' | 'auto'>('ssh');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.getHosts().then(setHosts).catch(() => {});
  }, []);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!username.trim()) {
      setError('Username is required');
      return;
    }

    const hostname = mode === 'host' ? undefined : adhocHostname.trim();
    if (mode === 'adhoc' && !hostname) {
      setError('Hostname is required');
      return;
    }

    setSubmitting(true);
    try {
      const req: CreateSessionRequest = {
        username,
        transport: transport === 'auto' ? 'ssh' : 'ssh',
        row: suggestedRow ?? 0,
        col: suggestedCol ?? 0,
      };

      if (mode === 'host' && selectedHostId) {
        req.host_id = selectedHostId;
      } else {
        req.hostname = hostname;
        req.port = adhocPort;
      }

      if (authMode === 'password' && password) {
        req.password = password;
      }

      onConnect(req);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={styles.backdrop} onClick={e => e.target === e.currentTarget && onClose()}>
      <div style={styles.dialog}>
        <div style={styles.header}>
          <span style={styles.title}>New SSH Session</span>
          <button style={styles.closeBtn} onClick={onClose}>✕</button>
        </div>

        <form onSubmit={handleSubmit} style={styles.form}>
          {/* Target host */}
          <div style={styles.section}>
            <div style={styles.tabs}>
              <button
                type="button"
                style={{ ...styles.tab, ...(mode === 'host' ? styles.tabActive : {}) }}
                onClick={() => setMode('host')}
              >
                Saved Host
              </button>
              <button
                type="button"
                style={{ ...styles.tab, ...(mode === 'adhoc' ? styles.tabActive : {}) }}
                onClick={() => setMode('adhoc')}
              >
                Ad-hoc
              </button>
            </div>

            {mode === 'host' ? (
              <select
                style={styles.input}
                value={selectedHostId}
                onChange={e => setSelectedHostId(e.target.value)}
              >
                <option value="">-- Select a host --</option>
                {hosts.map(h => (
                  <option key={h.id} value={h.id}>{h.id} ({h.hostname}:{h.port})</option>
                ))}
              </select>
            ) : (
              <div style={{ display: 'flex', gap: 8 }}>
                <input
                  style={{ ...styles.input, flex: 1 }}
                  type="text"
                  placeholder="hostname or IP"
                  value={adhocHostname}
                  onChange={e => setAdhocHostname(e.target.value)}
                />
                <input
                  style={{ ...styles.input, width: 70 }}
                  type="number"
                  placeholder="22"
                  value={adhocPort}
                  onChange={e => setAdhocPort(Number(e.target.value))}
                  min={1}
                  max={65535}
                />
              </div>
            )}
          </div>

          {/* Username */}
          <div style={styles.field}>
            <label style={styles.label}>Remote Username</label>
            <input
              style={styles.input}
              type="text"
              placeholder="user"
              value={username}
              onChange={e => setUsername(e.target.value)}
              autoComplete="username"
            />
          </div>

          {/* Auth mode */}
          <div style={styles.field}>
            <label style={styles.label}>Authentication</label>
            <div style={styles.tabs}>
              <button
                type="button"
                style={{ ...styles.tab, ...(authMode === 'password' ? styles.tabActive : {}) }}
                onClick={() => setAuthMode('password')}
              >
                Password
              </button>
              <button
                type="button"
                style={{ ...styles.tab, ...(authMode === 'key' ? styles.tabActive : {}) }}
                onClick={() => setAuthMode('key')}
              >
                Key
              </button>
            </div>

            {authMode === 'password' ? (
              <input
                style={styles.input}
                type="password"
                placeholder="Remote password (not stored)"
                value={password}
                onChange={e => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            ) : (
              <p style={styles.hint}>SSH key auth uses the jump box&apos;s default key identity.</p>
            )}
          </div>

          {/* Transport */}
          <div style={styles.field}>
            <label style={styles.label}>Transport</label>
            <select
              style={styles.input}
              value={transport}
              onChange={e => setTransport(e.target.value as 'ssh' | 'auto')}
            >
              <option value="ssh">SSH</option>
              <option value="auto">Auto (SSH + Mosh if available)</option>
            </select>
          </div>

          {error && <div style={styles.error}>{error}</div>}

          <div style={styles.actions}>
            <button type="button" style={styles.cancelBtn} onClick={onClose}>Cancel</button>
            <button type="submit" style={styles.connectBtn} disabled={submitting}>
              {submitting ? 'Connecting…' : 'Connect'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  backdrop: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.7)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
  },
  dialog: {
    background: '#1a1a2e',
    border: '1px solid #333366',
    borderRadius: 8,
    width: 460,
    maxHeight: '90vh',
    overflow: 'auto',
    boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #333366',
  },
  title: {
    fontWeight: 700,
    fontSize: 15,
    color: '#e0e0e0',
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#888',
    fontSize: 14,
    cursor: 'pointer',
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: 16,
  },
  section: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  label: {
    fontSize: 11,
    color: '#aaa',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  tabs: {
    display: 'flex',
    gap: 4,
  },
  tab: {
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '4px 12px',
    color: '#888',
    fontSize: 12,
    cursor: 'pointer',
  },
  tabActive: {
    background: '#2a2a4a',
    borderColor: '#7c6af7',
    color: '#e0e0e0',
  },
  input: {
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '7px 10px',
    color: '#e0e0e0',
    fontSize: 13,
    outline: 'none',
    width: '100%',
  },
  hint: {
    color: '#888',
    fontSize: 12,
    margin: 0,
    fontStyle: 'italic',
  },
  error: {
    background: '#3a1a1a',
    border: '1px solid #c04040',
    borderRadius: 4,
    padding: '7px 10px',
    color: '#ff8080',
    fontSize: 12,
  },
  actions: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: 8,
    paddingTop: 4,
  },
  cancelBtn: {
    background: '#1a1a3a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '7px 16px',
    color: '#aaa',
    fontSize: 13,
    cursor: 'pointer',
  },
  connectBtn: {
    background: '#7c6af7',
    border: 'none',
    borderRadius: 4,
    padding: '7px 20px',
    color: '#fff',
    fontSize: 13,
    fontWeight: 600,
    cursor: 'pointer',
  },
};
