import { useState, FormEvent } from 'react';
import { api } from '../utils/api';

interface RegisterDialogProps {
  onClose: () => void;
  onCreated: (username: string) => void;
}

export function RegisterDialog({ onClose, onCreated }: RegisterDialogProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!username.trim()) { setError('Username is required'); return; }
    if (username.trim().length < 2) { setError('Username must be at least 2 characters'); return; }
    if (!password) { setError('Password is required'); return; }
    if (password.length < 4) { setError('Password must be at least 4 characters'); return; }
    if (password !== confirmPassword) { setError('Passwords do not match'); return; }

    setSubmitting(true);
    try {
      await api.register(username.trim(), password);
      onCreated(username.trim());
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
          <span style={styles.title}>Create New Account</span>
          <button style={styles.closeBtn} onClick={onClose}>{'\u2715'}</button>
        </div>

        <p style={styles.hint}>
          Each account has its own collection of sessions.
          Use separate accounts to organize workspaces (e.g. home, work).
        </p>

        <form onSubmit={handleSubmit} style={styles.form}>
          <div style={styles.field}>
            <label style={styles.label}>Username</label>
            <input
              style={styles.input}
              type="text"
              placeholder="work"
              value={username}
              onChange={e => setUsername(e.target.value)}
              autoFocus
              autoComplete="off"
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label}>Password</label>
            <input
              style={styles.input}
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              autoComplete="new-password"
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label}>Confirm Password</label>
            <input
              style={styles.input}
              type="password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              autoComplete="new-password"
            />
          </div>

          {error && <div style={styles.error}>{error}</div>}

          <div style={styles.actions}>
            <button type="button" style={styles.cancelBtn} onClick={onClose}>Cancel</button>
            <button type="submit" style={styles.submitBtn} disabled={submitting}>
              {submitting ? 'Creating\u2026' : 'Create Account'}
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
    width: 380,
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
  hint: {
    color: '#777',
    fontSize: 12,
    margin: 0,
    padding: '10px 16px 0',
    lineHeight: 1.5,
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
