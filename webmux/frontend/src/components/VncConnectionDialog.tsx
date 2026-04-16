import { useState, useEffect, FormEvent } from 'react';
import { api } from '../utils/api';
import type { HostEntry, CreateVncSessionRequest } from '../types';

interface VncConnectionDialogProps {
  onConnect: (req: CreateVncSessionRequest, password: string) => Promise<void>;
  onClose: () => void;
  suggestedRow?: number;
  suggestedCol?: number;
}

export function VncConnectionDialog({ onConnect, onClose, suggestedRow, suggestedCol }: VncConnectionDialogProps) {
  const [vncHosts, setVncHosts] = useState<HostEntry[]>([]);
  const [hostname, setHostname] = useState('');
  const [vncPort, setVncPort] = useState(5900);
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.getHosts()
      .then(hosts => setVncHosts(hosts.filter(h => h.vnc_enabled)))
      .catch(() => {});
  }, []);

  const validate = (): boolean => {
    setError(null);
    if (!hostname.trim()) { setError('Hostname is required'); return false; }
    return true;
  };

  const buildRequest = (): CreateVncSessionRequest => ({
    hostname: hostname.trim(),
    vnc_port: vncPort,
    row: suggestedRow ?? 0,
    col: suggestedCol ?? 0,
  });

  const handleConnect = async (e: FormEvent) => {
    e.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    try {
      await onConnect(buildRequest(), password);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleQuickConnect = async (host: HostEntry) => {
    setSubmitting(true);
    setError(null);
    try {
      const req: CreateVncSessionRequest = {
        host_id: host.id,
        vnc_port: host.vnc_port || vncPort,
        row: suggestedRow ?? 0,
        col: suggestedCol ?? 0,
      };
      await onConnect(req, password);
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
          <span style={styles.title}>Connect to VNC Desktop</span>
          <button style={styles.closeBtn} onClick={onClose}>{'\u2715'}</button>
        </div>

        <form onSubmit={handleConnect} style={styles.form} autoComplete="off">
          {/* Saved VNC-enabled hosts as quick-connect cards */}
          {vncHosts.length > 0 && (
            <div style={styles.field}>
              <label style={styles.label}>Saved VNC Hosts</label>
              <div style={styles.hostGrid}>
                {vncHosts.map(h => (
                  <button
                    key={h.id}
                    type="button"
                    style={styles.hostCard}
                    onClick={() => handleQuickConnect(h)}
                    title={`VNC connect to ${h.hostname}:${h.vnc_port || 5900}`}
                    disabled={submitting}
                  >
                    <span style={styles.hostCardName}>{h.hostname}</span>
                    <span style={styles.hostCardPort}>:{h.vnc_port || 5900}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Divider when saved hosts exist */}
          {vncHosts.length > 0 && (
            <div style={styles.divider}>
              <span style={styles.dividerText}>or connect manually</span>
            </div>
          )}

          {/* Hostname */}
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
                data-1p-ignore
              />
              <input
                style={{ ...styles.input, width: 80 }}
                type="number"
                placeholder="5900"
                value={vncPort}
                onChange={e => setVncPort(Number(e.target.value))}
                min={1}
                max={65535}
              />
            </div>
          </div>

          {/* VNC Password */}
          <div style={styles.field}>
            <label style={styles.label}>VNC Password (optional)</label>
            <input
              style={styles.input}
              type="password"
              placeholder="Leave blank if none"
              value={password}
              onChange={e => setPassword(e.target.value)}
              autoComplete="off"
              data-1p-ignore
            />
          </div>

          {error && <div style={styles.error}>{error}</div>}

          <div style={styles.actions}>
            <button type="button" style={styles.cancelBtn} onClick={onClose}>Cancel</button>
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
    width: 400,
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
    gap: 2,
    background: '#0d0d1a',
    border: '1px solid #333366',
    borderRadius: 6,
    padding: '6px 10px',
    color: '#c0c0e0',
    fontSize: 13,
    cursor: 'pointer',
  },
  hostCardName: {
    fontWeight: 500,
  },
  hostCardPort: {
    color: '#666',
    fontSize: 11,
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
  input: {
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '7px 10px',
    color: '#e0e0e0',
    fontSize: 13,
    outline: 'none',
    width: '100%',
    boxSizing: 'border-box',
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
