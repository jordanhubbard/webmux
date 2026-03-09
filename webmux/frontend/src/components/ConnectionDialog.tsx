import { useState, useEffect, FormEvent } from 'react';
import { api } from '../utils/api';
import type { HostEntry, KeyEntry, CreateSessionRequest } from '../types';

interface ConnectionDialogProps {
  onConnect: (req: CreateSessionRequest) => Promise<void>;
  onClose: () => void;
  suggestedRow?: number;
  suggestedCol?: number;
}

export function ConnectionDialog({ onConnect, onClose, suggestedRow, suggestedCol }: ConnectionDialogProps) {
  const [hosts, setHosts] = useState<HostEntry[]>([]);
  const [keys, setKeys] = useState<Pick<KeyEntry, 'id' | 'type' | 'encrypted' | 'description'>[]>([]);
  const [hostname, setHostname] = useState('');
  const [port, setPort] = useState(22);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState<'agent' | 'password' | 'key'>('agent');
  const [selectedKeyId, setSelectedKeyId] = useState('');
  const [transport, setTransport] = useState<'ssh' | 'mosh'>('ssh');
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.getHosts().then(setHosts).catch(() => {});
    api.getKeys().then(setKeys).catch(() => {});
  }, []);

  const validate = (): boolean => {
    setError(null);
    if (!hostname.trim()) { setError('Hostname is required'); return false; }
    if (!username.trim()) { setError('Username is required'); return false; }
    return true;
  };

  const buildRequest = (): CreateSessionRequest => {
    const req: CreateSessionRequest = {
      username: username.trim(),
      hostname: hostname.trim(),
      port,
      transport,
      row: suggestedRow ?? 0,
      col: suggestedCol ?? 0,
    };
    if (authMode === 'password' && password) req.password = password;
    else if (authMode === 'key' && selectedKeyId) req.key_id = selectedKeyId;
    return req;
  };

  const handleConnect = async (e: FormEvent) => {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    try {
      await onConnect(buildRequest());
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleSaveAndConnect = async () => {
    if (!validate()) return;
    setSubmitting(true);
    try {
      const saved = await api.createHost({
        hostname: hostname.trim(),
        port,
        username: username.trim(),
        transport,
        key_id: authMode === 'key' ? selectedKeyId : '',
        tags: [],
        mosh_allowed: transport === 'mosh',
      });
      const req = buildRequest();
      req.host_id = saved.id;
      await onConnect(req);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleQuickConnect = async (host: HostEntry) => {
    const user = host.username || username.trim();
    if (!user) {
      setError('Enter a username first, then click a saved host to connect');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const req: CreateSessionRequest = {
        username: user,
        host_id: host.id,
        hostname: host.hostname,
        port: host.port,
        transport: host.transport || 'ssh',
        row: suggestedRow ?? 0,
        col: suggestedCol ?? 0,
      };
      if (host.key_id) req.key_id = host.key_id;
      await onConnect(req);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteHost = async (hostId: string) => {
    try {
      await api.deleteHost(hostId);
      setHosts(prev => prev.filter(h => h.id !== hostId));
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const alreadySaved = hosts.some(h => h.hostname === hostname.trim() && h.port === port && (!h.username || h.username === username.trim()));

  return (
    <div style={styles.backdrop} onClick={e => e.target === e.currentTarget && onClose()}>
      <div style={styles.dialog}>
        <div style={styles.header}>
          <span style={styles.title}>Connect to Host</span>
          <button style={styles.closeBtn} onClick={onClose}>{'\u2715'}</button>
        </div>

        <form onSubmit={handleConnect} style={styles.form}>
          {/* Saved hosts as quick-connect cards */}
          {hosts.length > 0 && (
            <div style={styles.field}>
              <label style={styles.label}>Saved Hosts</label>
              <div style={styles.hostGrid}>
                {hosts.map(h => (
                  <div key={h.id} style={styles.hostCard}>
                    <button
                      type="button"
                      style={styles.hostCardBtn}
                      onClick={() => handleQuickConnect(h)}
                      title={`Connect to ${h.username ? h.username + '@' : ''}${h.hostname}`}
                      disabled={submitting}
                    >
                      {h.username && <span style={styles.hostCardUser}>{h.username}@</span>}
                      <span style={styles.hostCardName}>{h.hostname}</span>
                      {h.port !== 22 && <span style={styles.hostCardPort}>:{h.port}</span>}
                    </button>
                    <button
                      type="button"
                      style={styles.hostDeleteBtn}
                      onClick={() => handleDeleteHost(h.id)}
                      title="Remove saved host"
                    >
                      {'\u2715'}
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Divider when hosts exist */}
          {hosts.length > 0 && (
            <div style={styles.divider}>
              <span style={styles.dividerText}>or connect to a new host</span>
            </div>
          )}

          {/* Hostname + Port */}
          <div style={styles.field}>
            <label style={styles.label}>Host</label>
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                style={{ ...styles.input, flex: 1 }}
                type="text"
                placeholder="hostname or IP"
                value={hostname}
                onChange={e => setHostname(e.target.value)}
                autoFocus
              />
              <input
                style={{ ...styles.input, width: 70 }}
                type="number"
                placeholder="22"
                value={port}
                onChange={e => setPort(Number(e.target.value))}
                min={1}
                max={65535}
              />
            </div>
          </div>

          {/* Username */}
          <div style={styles.field}>
            <label style={styles.label}>Username</label>
            <input
              style={styles.input}
              type="text"
              placeholder="user"
              value={username}
              onChange={e => setUsername(e.target.value)}
              autoComplete="username"
            />
          </div>

          {/* Advanced toggle */}
          <button
            type="button"
            style={styles.advancedToggle}
            onClick={() => setShowAdvanced(!showAdvanced)}
          >
            {showAdvanced ? '\u25be' : '\u25b8'} Advanced options
          </button>

          {showAdvanced && (
            <div style={styles.advanced}>
              <div style={styles.field}>
                <label style={styles.label}>Authentication</label>
                <div style={styles.tabs}>
                  <button type="button" style={{ ...styles.tab, ...(authMode === 'agent' ? styles.tabActive : {}) }} onClick={() => setAuthMode('agent')}>Agent</button>
                  <button type="button" style={{ ...styles.tab, ...(authMode === 'key' ? styles.tabActive : {}) }} onClick={() => setAuthMode('key')}>Key</button>
                  <button type="button" style={{ ...styles.tab, ...(authMode === 'password' ? styles.tabActive : {}) }} onClick={() => setAuthMode('password')}>Password</button>
                </div>
                {authMode === 'agent' && <p style={styles.hint}>Uses the SSH agent or default keys (~/.ssh/id_*). No credentials needed.</p>}
                {authMode === 'key' && (
                  <>
                    <select style={styles.input} value={selectedKeyId} onChange={e => setSelectedKeyId(e.target.value)}>
                      <option value="">Default key (agent / ~/.ssh/id_*)</option>
                      {keys.map(k => <option key={k.id} value={k.id}>{k.description || k.id} ({k.type}{k.encrypted ? ', encrypted' : ''})</option>)}
                    </select>
                    <p style={styles.hint}>Select a specific key from keys.yaml.</p>
                  </>
                )}
                {authMode === 'password' && (
                  <input style={styles.input} type="password" placeholder="Remote password (requires sshpass)" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" />
                )}
              </div>
              <div style={styles.field}>
                <label style={styles.label}>Transport</label>
                <select style={styles.input} value={transport} onChange={e => setTransport(e.target.value as 'ssh' | 'mosh')}>
                  <option value="ssh">SSH</option>
                  <option value="mosh">Mosh</option>
                </select>
              </div>
            </div>
          )}

          {error && <div style={styles.error}>{error}</div>}

          <div style={styles.actions}>
            <button type="button" style={styles.cancelBtn} onClick={onClose}>Cancel</button>
            {!alreadySaved && hostname.trim() && (
              <button type="button" style={styles.saveBtn} onClick={handleSaveAndConnect} disabled={submitting}>
                {submitting ? 'Saving\u2026' : 'Save & Connect'}
              </button>
            )}
            <button type="submit" style={styles.connectBtn} disabled={submitting}>
              {submitting ? 'Connecting\u2026' : 'Connect'}
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
    width: 420,
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
    gap: 14,
    padding: 16,
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
  hostGrid: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: 6,
  },
  hostCard: {
    display: 'flex',
    alignItems: 'center',
    background: '#0d0d1a',
    border: '1px solid #333366',
    borderRadius: 6,
    overflow: 'hidden',
  },
  hostCardBtn: {
    background: 'none',
    border: 'none',
    padding: '6px 10px',
    color: '#c0c0e0',
    fontSize: 13,
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    gap: 2,
  },
  hostCardUser: {
    color: '#888',
    fontSize: 12,
  },
  hostCardName: {
    fontWeight: 500,
  },
  hostCardPort: {
    color: '#666',
    fontSize: 11,
  },
  hostDeleteBtn: {
    background: 'none',
    border: 'none',
    borderLeft: '1px solid #2a2a4a',
    padding: '6px 8px',
    color: '#664444',
    fontSize: 10,
    cursor: 'pointer',
    lineHeight: 1,
  },
  divider: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
  },
  dividerText: {
    fontSize: 11,
    color: '#555',
    whiteSpace: 'nowrap',
    width: '100%',
    textAlign: 'center',
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
  advancedToggle: {
    background: 'none',
    border: 'none',
    color: '#777',
    fontSize: 12,
    cursor: 'pointer',
    textAlign: 'left',
    padding: 0,
  },
  advanced: {
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    paddingLeft: 8,
    borderLeft: '2px solid #2a2a4a',
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
  saveBtn: {
    background: '#2a4a3a',
    border: '1px solid #3a6a4a',
    borderRadius: 4,
    padding: '7px 14px',
    color: '#80cc90',
    fontSize: 13,
    fontWeight: 500,
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
