import { describe, it, expect, vi, beforeEach } from 'vitest';

function clearStorage() {
  try { localStorage.clear(); } catch { /* jsdom may not support */ }
  try { localStorage.removeItem('webmux_token'); } catch { /* fallback */ }
}

describe('api utilities', () => {
  beforeEach(() => {
    clearStorage();
    vi.resetModules();
  });

  describe('buildWsUrl', () => {
    it('builds ws:// URL for http origin', async () => {
      const { buildWsUrl } = await import('@frontend/utils/api');
      const url = buildWsUrl('session-123');
      expect(url).toContain('ws://');
      expect(url).toContain('/api/term/session-123');
    });

    it('includes token in query when present', async () => {
      localStorage.setItem('webmux_token', 'my-jwt-token');
      const { buildWsUrl } = await import('@frontend/utils/api');
      const url = buildWsUrl('s1');
      expect(url).toContain('?token=my-jwt-token');
      clearStorage();
    });

    it('omits token query when no token', async () => {
      const { buildWsUrl } = await import('@frontend/utils/api');
      const url = buildWsUrl('s1');
      expect(url).not.toContain('?token=');
    });
  });

  describe('api client', () => {
    let fetchSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      fetchSpy = vi.fn();
      global.fetch = fetchSpy;
      clearStorage();
    });

    it('getAuthStatus calls correct endpoint', async () => {
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ mode: 'none', bootstrap_required: false }),
      });

      const { api } = await import('@frontend/utils/api');
      const result = await api.getAuthStatus();
      expect(result.mode).toBe('none');
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/auth/status',
        expect.objectContaining({ headers: expect.any(Object) })
      );
    });

    it('createSession sends POST with body', async () => {
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 201,
        json: () => Promise.resolve({ id: 's1', hostname: 'h' }),
      });

      const { api } = await import('@frontend/utils/api');
      await api.createSession({ username: 'u', hostname: 'h' });
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/sessions',
        expect.objectContaining({ method: 'POST' })
      );
    });

    it('deleteSession sends DELETE', async () => {
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 204,
        json: () => Promise.reject(),
      });

      const { api } = await import('@frontend/utils/api');
      await api.deleteSession('s1');
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/sessions/s1',
        expect.objectContaining({ method: 'DELETE' })
      );
    });

    it('throws on non-ok response', async () => {
      fetchSpy.mockResolvedValue({
        ok: false,
        status: 500,
        statusText: 'Server Error',
        json: () => Promise.resolve({ error: 'Something failed' }),
      });

      const { api } = await import('@frontend/utils/api');
      await expect(api.getAuthStatus()).rejects.toThrow('Something failed');
    });

    it('includes auth header when token exists', async () => {
      localStorage.setItem('webmux_token', 'test-token');
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve([]),
      });

      const { api } = await import('@frontend/utils/api');
      await api.getSessions();
      const call = fetchSpy.mock.calls[0];
      expect(call[1].headers.Authorization).toBe('Bearer test-token');
      clearStorage();
    });

    it('getKeys calls correct endpoint', async () => {
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve([]),
      });

      const { api } = await import('@frontend/utils/api');
      await api.getKeys();
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/keys',
        expect.any(Object)
      );
    });
  });
});
