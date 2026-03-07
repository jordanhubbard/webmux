import { useState, FormEvent } from 'react';
import type { AuthState } from '../hooks/useAuth';

interface LoginPageProps {
  auth: AuthState;
}

export function LoginPage({ auth }: LoginPageProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const isBootstrap = auth.authStatus?.bootstrap_required ?? false;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLocalError(null);

    if (isBootstrap && password !== confirmPassword) {
      setLocalError('Passwords do not match');
      return;
    }
    if (!username.trim() || !password) {
      setLocalError('Username and password are required');
      return;
    }

    setSubmitting(true);
    try {
      if (isBootstrap) {
        await auth.bootstrap(username, password);
      } else {
        await auth.login(username, password);
      }
    } catch {
      // error is set in useAuth
    } finally {
      setSubmitting(false);
    }
  };

  const displayError = localError || auth.error;

  return (
    <div style={styles.overlay}>
      <div style={styles.card}>
        <div style={styles.logo}>
          <span style={styles.logoIcon}>▦</span>
          <span style={styles.logoText}>WebMux</span>
        </div>
        <p style={styles.subtitle}>
          {isBootstrap ? 'Create your admin account' : 'Sign in to your terminal wall'}
        </p>

        <form onSubmit={handleSubmit} style={styles.form}>
          <div style={styles.field}>
            <label style={styles.label} htmlFor="username">Username</label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              style={styles.input}
              placeholder="admin"
              autoComplete="username"
              disabled={submitting}
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label} htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              style={styles.input}
              autoComplete={isBootstrap ? 'new-password' : 'current-password'}
              disabled={submitting}
            />
          </div>

          {isBootstrap && (
            <div style={styles.field}>
              <label style={styles.label} htmlFor="confirm">Confirm Password</label>
              <input
                id="confirm"
                type="password"
                value={confirmPassword}
                onChange={e => setConfirmPassword(e.target.value)}
                style={styles.input}
                autoComplete="new-password"
                disabled={submitting}
              />
            </div>
          )}

          {displayError && (
            <div style={styles.error}>{displayError}</div>
          )}

          <button type="submit" style={styles.button} disabled={submitting}>
            {submitting ? 'Please wait…' : isBootstrap ? 'Create Account' : 'Sign In'}
          </button>
        </form>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '100%',
    height: '100%',
    background: '#0d0d1a',
  },
  card: {
    background: '#1a1a2e',
    border: '1px solid #333366',
    borderRadius: 8,
    padding: '2.5rem 2rem',
    width: 360,
    boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
  },
  logo: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    marginBottom: 8,
  },
  logoIcon: {
    fontSize: 28,
    color: '#7c6af7',
  },
  logoText: {
    fontSize: 22,
    fontWeight: 700,
    color: '#e0e0e0',
    letterSpacing: 1,
  },
  subtitle: {
    color: '#888',
    fontSize: 13,
    margin: '0 0 1.5rem',
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  label: {
    fontSize: 12,
    color: '#aaa',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  input: {
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '8px 12px',
    color: '#e0e0e0',
    fontSize: 14,
    outline: 'none',
  },
  error: {
    background: '#3a1a1a',
    border: '1px solid #c04040',
    borderRadius: 4,
    padding: '8px 12px',
    color: '#ff8080',
    fontSize: 13,
  },
  button: {
    background: '#7c6af7',
    border: 'none',
    borderRadius: 4,
    padding: '10px 16px',
    color: '#fff',
    fontSize: 14,
    fontWeight: 600,
    cursor: 'pointer',
    marginTop: 8,
  },
};
