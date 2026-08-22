import { useState, useEffect, useCallback } from 'react';
import { api } from '../utils/api';
import type { AuthStatus } from '../types';
import { AUTH_EXPIRED_EVENT, getTokenExpiration, signalAuthExpired } from '../utils/authSession';

export interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  authStatus: AuthStatus | null;
  error: string | null;
  username: string | null;
  isAdmin: boolean;
  sessionExpiresAt: number | null;
  sessionExpired: boolean;
  isRefreshing: boolean;
  login: (username: string, password: string) => Promise<void>;
  bootstrap: (username: string, password: string) => Promise<void>;
  refreshSession: () => Promise<void>;
  logout: () => void;
}

export function useAuth(): AuthState {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [username, setUsername] = useState<string | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [sessionExpiresAt, setSessionExpiresAt] = useState<number | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const storeToken = useCallback((token: string) => {
    localStorage.setItem('webmux_token', token);
    setSessionExpiresAt(getTokenExpiration(token));
    setSessionExpired(false);
  }, []);

  useEffect(() => {
    const expire = () => setSessionExpired(true);
    window.addEventListener(AUTH_EXPIRED_EVENT, expire);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, expire);
  }, []);

  useEffect(() => {
    if (!isAuthenticated || !sessionExpiresAt) return;
    const remaining = sessionExpiresAt - Date.now();
    if (remaining <= 0) {
      signalAuthExpired();
      return;
    }
    const timer = window.setTimeout(signalAuthExpired, remaining);
    return () => window.clearTimeout(timer);
  }, [isAuthenticated, sessionExpiresAt]);

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
            setSessionExpiresAt(getTokenExpiration(token));
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
      storeToken(token);
      setIsAuthenticated(true);
      await refreshMe();
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, [refreshMe, storeToken]);

  const bootstrap = useCallback(async (user: string, password: string) => {
    setError(null);
    try {
      const { token } = await api.bootstrap(user, password);
      storeToken(token);
      setIsAuthenticated(true);
      await refreshMe();
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, [refreshMe, storeToken]);

  const refreshSession = useCallback(async () => {
    setIsRefreshing(true);
    setError(null);
    try {
      const { token } = await api.refreshToken();
      storeToken(token);
    } catch (err) {
      setError((err as Error).message);
      throw err;
    } finally {
      setIsRefreshing(false);
    }
  }, [storeToken]);

  const logout = useCallback(() => {
    localStorage.removeItem('webmux_token');
    setIsAuthenticated(false);
    setUsername(null);
    setIsAdmin(false);
    setSessionExpiresAt(null);
    setSessionExpired(false);
    // Re-check server auth status so the login screen reflects reality after a
    // same-session sign-out. Without this, signing out right after first-run
    // setup would keep the stale bootstrap flag and wrongly show the
    // "First-time setup / Create Account" screen even though an account exists.
    api.getAuthStatus().then(setAuthStatus).catch(() => {});
  }, []);

  return {
    isAuthenticated, isLoading, authStatus, error, username, isAdmin,
    sessionExpiresAt, sessionExpired, isRefreshing,
    login, bootstrap, refreshSession, logout,
  };
}
