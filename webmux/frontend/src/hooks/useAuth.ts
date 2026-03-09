import { useState, useEffect, useCallback } from 'react';
import { api } from '../utils/api';
import type { AuthStatus } from '../types';

export interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  authStatus: AuthStatus | null;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  bootstrap: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

export function useAuth(): AuthState {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const status = await api.getAuthStatus();
        setAuthStatus(status);
        if (status.mode === 'none') {
          setIsAuthenticated(true);
        } else {
          const token = localStorage.getItem('webmux_token');
          if (token) {
            // Validate token by making an authenticated request
            try {
              await api.getSessions();
              setIsAuthenticated(true);
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
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    setError(null);
    try {
      const { token } = await api.login(username, password);
      localStorage.setItem('webmux_token', token);
      setIsAuthenticated(true);
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, []);

  const bootstrap = useCallback(async (username: string, password: string) => {
    setError(null);
    try {
      const { token } = await api.bootstrap(username, password);
      localStorage.setItem('webmux_token', token);
      setIsAuthenticated(true);
    } catch (err) {
      setError((err as Error).message);
      throw err;
    }
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem('webmux_token');
    setIsAuthenticated(false);
  }, []);

  return { isAuthenticated, isLoading, authStatus, error, login, bootstrap, logout };
}
