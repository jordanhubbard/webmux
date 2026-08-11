import { useState, useEffect, useCallback } from 'react';
import { api } from '../utils/api';
import type { AuthStatus } from '../types';

export interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  authStatus: AuthStatus | null;
  error: string | null;
  username: string | null;
  isAdmin: boolean;
  login: (username: string, password: string) => Promise<void>;
  bootstrap: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

export function useAuth(): AuthState {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [username, setUsername] = useState<string | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);

  // Loads the current user's identity and admin status. Failure is non-fatal —
  // the user stays authenticated but simply without management controls.
  const refreshMe = useCallback(async () => {
    try {
      const me = await api.getMe();
      setUsername(me.username);
      setIsAdmin(me.admin);
    } catch {
      setUsername(null);
      setIsAdmin(false);
    }
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const status = await api.getAuthStatus();
        setAuthStatus(status);
        if (status.mode === 'none') {
          setIsAuthenticated(true);
          await refreshMe();
        } else {
          const token = localStorage.getItem('webmux_token');
          if (token) {
            // Validate token by making an authenticated request
            try {
              await api.getSessions();
              setIsAuthenticated(true);
              await refreshMe();
            } catch {
              localStorage.removeItem('webmux_token');
            }
          }
        }
      } catch {
        setError('Failed to reach server');
      } finally {
        setIsLoading(false);
      }
    })();
  }, [refreshMe]);

  const login = useCallback(async (user: string, password: string) => {
    setError(null);
    try {
      const { token } = await api.login(user, password);
      localStorage.setItem('webmux_token', token);
      setIsAuthenticated(true);
      await refreshMe();
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, [refreshMe]);

  const bootstrap = useCallback(async (user: string, password: string) => {
    setError(null);
    try {
      const { token } = await api.bootstrap(user, password);
      localStorage.setItem('webmux_token', token);
      setIsAuthenticated(true);
      await refreshMe();
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, [refreshMe]);

  const logout = useCallback(() => {
    localStorage.removeItem('webmux_token');
    setIsAuthenticated(false);
    setUsername(null);
    setIsAdmin(false);
    // Re-check server auth status so the login screen reflects reality after a
    // same-session sign-out. Without this, signing out right after first-run
    // setup would keep the stale bootstrap flag and wrongly show the
    // "First-time setup / Create Account" screen even though an account exists.
    api.getAuthStatus().then(setAuthStatus).catch(() => {});
  }, []);

  return { isAuthenticated, isLoading, authStatus, error, username, isAdmin, login, bootstrap, logout };
}
