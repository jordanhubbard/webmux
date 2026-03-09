import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';

const mockApi = vi.hoisted(() => ({
  getAuthStatus: vi.fn(),
  getSessions: vi.fn().mockResolvedValue([]),
  login: vi.fn(),
  bootstrap: vi.fn(),
}));

vi.mock('@frontend/utils/api', () => ({
  api: mockApi,
}));

import { useAuth } from '@frontend/hooks/useAuth';

describe('useAuth', () => {
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
      try { await result.current.login('admin', 'wrong'); } catch {}
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
});
