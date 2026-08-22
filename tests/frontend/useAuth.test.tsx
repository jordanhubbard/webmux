import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';

const mockApi = vi.hoisted(() => ({
  getAuthStatus: vi.fn(),
  getSessions: vi.fn().mockResolvedValue([]),
  getMe: vi.fn().mockResolvedValue({ username: 'admin', admin: true }),
  login: vi.fn(),
  bootstrap: vi.fn(),
  refreshToken: vi.fn(),
}));

vi.mock('@frontend/utils/api', () => ({
  api: mockApi,
}));

import { useAuth } from '@frontend/hooks/useAuth';

describe('useAuth', () => {
  function tokenExpiringAt(expiresAt: number): string {
    return `header.${btoa(JSON.stringify({ sub: 'admin', exp: Math.floor(expiresAt / 1000) }))}.signature`;
  }

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.removeItem('webmux_token');
  });

  it('starts in loading state', () => {
    mockApi.getAuthStatus.mockReturnValue(new Promise(() => {})); // never resolves
    const { result } = renderHook(() => useAuth());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.isAuthenticated).toBe(false);
  });

  it('auto-authenticates in none mode', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'none', bootstrap_required: false });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.authStatus?.mode).toBe('none');
  });

  it('authenticates with existing token in local mode', async () => {
    localStorage.setItem('webmux_token', 'existing-token');
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.isAuthenticated).toBe(true);
    await waitFor(() => expect(result.current.username).toBe('admin'));
    expect(result.current.isAdmin).toBe(true);
    localStorage.removeItem('webmux_token');
  });

  it('is not authenticated without token in local mode', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.isAuthenticated).toBe(false);
  });

  it('sets error when server unreachable', async () => {
    mockApi.getAuthStatus.mockRejectedValue(new Error('Network error'));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.error).toBe('Failed to reach server');
  });

  it('login stores token and sets authenticated', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    mockApi.login.mockResolvedValue({ token: 'new-token', mode: 'local' });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      await result.current.login('admin', 'pass');
    });
    expect(result.current.isAuthenticated).toBe(true);
    expect(localStorage.getItem('webmux_token')).toBe('new-token');
    localStorage.removeItem('webmux_token');
  });

  it('login sets error on failure', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    mockApi.login.mockRejectedValue(new Error('Invalid credentials'));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      try { await result.current.login('admin', 'wrong'); } catch { /* expected rejection */ }
    });
    expect(result.current.error).toBe('Invalid credentials');
    expect(result.current.isAuthenticated).toBe(false);
  });

  it('bootstrap stores token and sets authenticated', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: true });
    mockApi.bootstrap.mockResolvedValue({ token: 'bootstrap-token', mode: 'local' });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      await result.current.bootstrap('admin', 'newpass');
    });
    expect(result.current.isAuthenticated).toBe(true);
    expect(localStorage.getItem('webmux_token')).toBe('bootstrap-token');
    localStorage.removeItem('webmux_token');
  });

  it('refreshes the token and updates its expiration', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    const initialToken = tokenExpiringAt(Date.now() + 60_000);
    const expectedExpiration = Math.floor((Date.now() + 8 * 60 * 60 * 1000) / 1000) * 1000;
    const refreshedToken = tokenExpiringAt(expectedExpiration);
    localStorage.setItem('webmux_token', initialToken);
    mockApi.refreshToken.mockResolvedValue({ token: refreshedToken, mode: 'local' });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => result.current.refreshSession());

    expect(localStorage.getItem('webmux_token')).toBe(refreshedToken);
    expect(result.current.sessionExpiresAt).toBe(expectedExpiration);
  });

  it('marks the session expired when notified of an authentication failure', async () => {
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    localStorage.setItem('webmux_token', tokenExpiringAt(Date.now() + 60_000));
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => window.dispatchEvent(new Event('webmux:auth-expired')));

    expect(result.current.sessionExpired).toBe(true);
  });

  it('logout clears token and sets unauthenticated', async () => {
    localStorage.setItem('webmux_token', 'some-token');
    mockApi.getAuthStatus.mockResolvedValue({ mode: 'local', bootstrap_required: false });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      result.current.logout();
    });
    expect(result.current.isAuthenticated).toBe(false);
    expect(localStorage.getItem('webmux_token')).toBeNull();
  });

  it('logout re-fetches auth status so a stale bootstrap flag is cleared', async () => {
    // Mount with bootstrap_required:true (no accounts yet), then the server
    // reports an account exists after bootstrap. Signing out must refresh the
    // flag so the login screen shows sign-in, not the first-run setup.
    mockApi.getAuthStatus
      .mockResolvedValueOnce({ mode: 'local', bootstrap_required: true })
      .mockResolvedValue({ mode: 'local', bootstrap_required: false });
    const { result } = renderHook(() => useAuth());
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.authStatus?.bootstrap_required).toBe(true);

    await act(async () => {
      result.current.logout();
    });
    await waitFor(() => expect(result.current.authStatus?.bootstrap_required).toBe(false));
  });
});
