import { useState, useEffect, useCallback, FormEvent } from 'react';
import { api } from '../utils/api';
import type { AuthUserInfo } from '../types';

interface UsersDialogProps {
  currentUser: string | null;
  onClose: () => void;
}

// Admin-only account management: list existing accounts, add new ones, and
// remove accounts. Guardrails (can't remove yourself or the last admin) are
// enforced by the backend; the UI mirrors them by disabling the affected rows.
export function UsersDialog({ currentUser, onClose }: UsersDialogProps) {
  const [users, setUsers] = useState<AuthUserInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  // Add-account form state
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [makeAdmin, setMakeAdmin] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      setUsers(await api.getUsers());
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const name = username.trim();
    if (name.length < 2) { setError('Username must be at least 2 characters'); return; }
    if (password.length < 4) { setError('Password must be at least 4 characters'); return; }
    if (password !== confirmPassword) { setError('Passwords do not match'); return; }

    setSubmitting(true);
    try {
      await api.register(name, password, makeAdmin);
      setUsername('');
      setPassword('');
      setConfirmPassword('');
      setMakeAdmin(false);
      await load();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleRemove = async (name: string) => {
    setError(null);
    if (!window.confirm(`Remove account "${name}"? This cannot be undone. Their sessions become inaccessible.`)) {
      return;
    }
    setBusy(name);
    try {
      await api.deleteUser(name);
      await load();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div style={styles.backdrop} onClick={e => e.target === e.currentTarget && onClose()}>
      <div style={styles.dialog}>
        <div style={styles.header}>
          <span style={styles.title}>Manage Accounts</span>
          <button style={styles.closeBtn} onClick={onClose}>{'✕'}</button>
        </div>

        <p style={styles.hint}>
          Each account has its own collection of sessions. Admins can add and remove
          accounts. Guests can sign in but cannot manage accounts.
        </p>

        <div style={styles.listWrap}>
          {loading ? (
            <div style={styles.muted}>Loading…</div>
          ) : users.length === 0 ? (
            <div style={styles.muted}>No accounts.</div>
          ) : (
            users.map(u => {
              const isSelf = u.username === currentUser;
              const disabled = isSelf || busy === u.username;
              const reason = isSelf ? 'This is you' : '';
              return (
                <div key={u.username} style={styles.row}>
                  <div style={styles.rowMain}>
                    <span style={styles.rowName}>{u.username}</span>
                    {u.admin && <span style={styles.adminBadge}>admin</span>}
                    {isSelf && <span style={styles.selfBadge}>you</span>}
                  </div>
                  <button
                    style={{ ...styles.removeBtn, ...(disabled ? styles.removeBtnDisabled : {}) }}
                    onClick={() => handleRemove(u.username)}
                    disabled={disabled}
                    title={reason || `Remove ${u.username}`}
                  >
                    {busy === u.username ? 'Removing…' : reason || 'Remove'}
                  </button>
                </div>
              );
            })
          )}
        </div>

        <form onSubmit={handleAdd} style={styles.form}>
          <div style={styles.formTitle}>Add account</div>
          <div style={styles.fieldRow}>
            <input
              style={styles.input}
              type="text"
              placeholder="username"
              value={username}
              onChange={e => setUsername(e.target.value)}
              autoComplete="off"
            />
          </div>
          <div style={styles.fieldRow}>
            <input
              style={styles.input}
              type="password"
              placeholder="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              autoComplete="new-password"
            />
            <input
              style={styles.input}
              type="password"
              placeholder="confirm"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              autoComplete="new-password"
            />
          </div>
          <label style={styles.checkRow}>
            <input type="checkbox" checked={makeAdmin} onChange={e => setMakeAdmin(e.target.checked)} />
            Grant admin (can manage accounts)
          </label>

          {error && <div style={styles.error}>{error}</div>}

          <div style={styles.actions}>
            <button type="button" style={styles.cancelBtn} onClick={onClose}>Close</button>
            <button type="submit" style={styles.submitBtn} disabled={submitting}>
              {submitting ? 'Adding…' : 'Add Account'}
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
    maxHeight: '85vh',
    display: 'flex',
    flexDirection: 'column',
    boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #333366',
  },
  title: { fontWeight: 700, fontSize: 15, color: '#e0e0e0' },
  closeBtn: { background: 'none', border: 'none', color: '#888', fontSize: 14, cursor: 'pointer' },
  hint: { color: '#777', fontSize: 12, margin: 0, padding: '10px 16px 0', lineHeight: 1.5 },
  listWrap: {
    padding: '10px 16px',
    overflowY: 'auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  muted: { color: '#777', fontSize: 13, padding: '4px 0' },
  row: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    background: '#0d0d1a',
    border: '1px solid #2a2a44',
    borderRadius: 4,
    padding: '8px 10px',
  },
  rowMain: { display: 'flex', alignItems: 'center', gap: 8 },
  rowName: { color: '#e0e0e0', fontSize: 13 },
  adminBadge: {
    fontSize: 10,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    color: '#9d8cff',
    border: '1px solid #4a3a8a',
    borderRadius: 3,
    padding: '1px 5px',
  },
  selfBadge: {
    fontSize: 10,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    color: '#4aaa6a',
    border: '1px solid #2a6a4a',
    borderRadius: 3,
    padding: '1px 5px',
  },
  removeBtn: {
    background: '#3a1a1a',
    border: '1px solid #6a2a2a',
    borderRadius: 4,
    padding: '4px 10px',
    color: '#ff8080',
    fontSize: 12,
    cursor: 'pointer',
  },
  removeBtnDisabled: {
    background: '#1a1a2a',
    border: '1px solid #2a2a3a',
    color: '#666',
    cursor: 'not-allowed',
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: 16,
    borderTop: '1px solid #333366',
  },
  formTitle: {
    fontSize: 11,
    color: '#aaa',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  fieldRow: { display: 'flex', gap: 8 },
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
  checkRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    color: '#aaa',
    fontSize: 12,
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
  submitBtn: {
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
