import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

function clearStorage() {
  try { localStorage.clear(); } catch { /* jsdom may not support */ }
  try { localStorage.removeItem('webmux_token'); } catch { /* fallback */ }
}

describe('api utilities', () => {
  afterEach(() => { vi.unstubAllGlobals(); });
  beforeEach(() => {
    clearStorage();
    vi.resetModules();
  });

  it('rejects malformed coordination policies instead of treating an unknown state as disabled', async () => {
    const { api } = await import('@frontend/utils/api');
    const fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock);
    for (const value of [null, {}, { enabled: false }, { contract: 'webmux-session-coordination-policy/v1', enabled: 'false' }]) {
      fetchMock.mockResolvedValueOnce({ ok: true, status: 200, json: async () => value });
      await expect(api.getSessionCoordinationPolicy()).rejects.toThrow('invalid coordination policy');
    }
    const value = { contract: 'webmux-session-coordination-policy/v1', enabled: false };
    fetchMock.mockResolvedValueOnce({ ok: true, status: 200, json: async () => value });
    await expect(api.getSessionCoordinationPolicy()).resolves.toEqual(value);
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

  it('routes a namespaced session through this server without exposing a peer credential', async () => {
    localStorage.setItem('webmux_token', 'browser-token');
    const { buildWsUrl } = await import('@frontend/utils/api');
    expect(buildWsUrl('@remote/same-id')).toContain('/api/servers/remote/term/same-id?token=browser-token');
    expect(buildWsUrl('same-id')).toContain('/api/term/same-id?token=browser-token');
    expect(buildWsUrl('@remote/../api/config')).not.toContain('/api/servers/remote/');
  });

  describe('api client', () => {
    let fetchSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      fetchSpy = vi.fn();
      vi.stubGlobal('fetch', fetchSpy);
      clearStorage();
    });

    it('preserves tracked uncertainty for lost, malformed and ambiguous replies without a fallback POST', async () => {
      const { api, CoordinationCreationUnconfirmedError } = await import('@frontend/utils/api');
      const input = { provider: 'rig', mode: 'new' as const, projectName: 'project', name: 'coder', host: 'localhost', workspace: '/work', runtime: 'codex', creationRequestId: '2d438c43-c962-4b1d-b8ed-a35e07a8e964', row: 1, col: 2 };
      const operation = { state: 'uncertain', stage: 'agent', code: 'native_create_response_unavailable', operationId: '11111111-1111-4111-8111-111111111111' };
      const cases = [
        () => Promise.reject(new Error('lost reply')),
        () => Promise.resolve({ ok: true, status: 200, json: async () => { throw new Error('truncated JSON'); } }),
        ...[null, {}, { session: {} }, { creation: { ...operation, operationId: 'foreign' } }, { creation: operation, session: { id: 'ambiguous' } }]
          .map(body => () => Promise.resolve({ ok: true, status: 200, json: async () => body })),
        () => Promise.resolve({ ok: false, status: 502, json: async () => ({ error: 'unknown failure' }) }),
        () => Promise.resolve({ ok: false, status: 409, json: async () => ({ error: 'viewer full after creation' }) }),
      ];
      for (const reply of cases) {
        fetchSpy.mockImplementationOnce(reply);
        await expect(api.createCoordinatedAgent(input)).rejects.toBeInstanceOf(CoordinationCreationUnconfirmedError);
      }
      expect(fetchSpy).toHaveBeenCalledTimes(cases.length);
      for (const call of fetchSpy.mock.calls) {
        expect(call[0]).toBe('/api/coordination/creation-requests/' + input.creationRequestId);
        expect(JSON.parse(call[1].body)).toEqual(input);
      }
    });

    it('accepts confirmed tracked replies and clears only a first explicit pre-dispatch refusal', async () => {
      const { api, CoordinationCreationUnconfirmedError } = await import('@frontend/utils/api');
      const input = { provider: 'rig', mode: 'new' as const, projectName: 'project', name: 'coder', host: 'localhost', workspace: '/work', runtime: 'codex', creationRequestId: '2d438c43-c962-4b1d-b8ed-a35e07a8e964' };
      for (const result of [{ target: 'original', session: { id: 'viewer' } }, { creation: { state: 'uncertain', stage: 'agent', code: 'native_create_response_unavailable', operationId: '11111111-1111-4111-8111-111111111111' } }]) {
        fetchSpy.mockResolvedValueOnce({ ok: true, status: 200, json: async () => result });
        await expect(api.createCoordinatedAgent(input)).resolves.toEqual(result);
      }
      fetchSpy.mockResolvedValue({ ok: false, status: 400, json: async () => ({ error: 'preflight refused', refused: { state: 'refused', code: 'invalid_creation_request' } }) });
      const initial = await api.createCoordinatedAgent(input).catch(error => error);
      expect(initial).not.toBeInstanceOf(CoordinationCreationUnconfirmedError);
      expect(String(initial)).toContain('preflight refused');
      for (const continuation of [{ continueOriginal: true }, { inspect: true }, { trustRequest: 'a'.repeat(64) }]) {
        await expect(api.createCoordinatedAgent({ ...input, ...continuation })).rejects.toBeInstanceOf(CoordinationCreationUnconfirmedError);
      }
    });

    it('types a local native trust refusal while preserving the selected retry token', async () => {
      const { api, CoordinationTrustRequiredError } = await import('@frontend/utils/api');
      const requestId = 'a'.repeat(64);
      const input = { provider: 'rig', mode: 'new' as const, projectName: 'project', name: 'coder', host: 'localhost', workspace: '/work', runtime: 'codex', trustRequest: requestId };
      fetchSpy.mockResolvedValue({ ok: false, status: 409, json: async () => ({ error: 'Review trust', blocked: { state: 'blocked', code: 'native_project_trust_required', requestId } }) });
      const error = await api.createCoordinatedAgent(input).catch(value => value);
      expect(error).toBeInstanceOf(CoordinationTrustRequiredError);
      expect(error.requestId).toBe(requestId);
      expect(JSON.parse(fetchSpy.mock.calls[0][1].body)).toEqual(input);
    });

    it('does not reinterpret malformed, ambiguous, remote or unrelated failures as native trust authority', async () => {
      const { api, CoordinationTrustRequiredError } = await import('@frontend/utils/api');
      const input = { provider: 'rig', mode: 'new' as const, projectName: 'project', name: 'coder', host: 'localhost', workspace: '/work', runtime: 'codex' };
      const blocked = { state: 'blocked', code: 'native_project_trust_required', requestId: 'a'.repeat(64) };
      for (const [status, body, selected] of [
        [502, { error: 'native failure', blocked }, input],
        [409, { error: 'native failure', blocked: { ...blocked, requestId: '../foreign' } }, input],
        [409, { error: 'native failure', blocked: { ...blocked, code: 'foreign' } }, input],
        [409, { error: 'native failure', blocked, session: {} }, input],
        [409, { error: 'native failure', blocked }, { ...input, host: 'remote' }],
        [409, { error: 'native failure', blocked }, { ...input, provider: 'agent-deck' }],
      ] as const) {
        fetchSpy.mockResolvedValue({ ok: false, status, json: async () => body });
        const error = await api.createCoordinatedAgent(selected).catch(value => value);
        expect(error).not.toBeInstanceOf(CoordinationTrustRequiredError);
        expect(String(error)).toContain('native failure');
      }
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

    it('updateConfig sends PUT with config body', async () => {
      const returnedConfig = {
        app: { name: 'webmux', default_term: { cols: 100, rows: 30, font_size: 16 } },
      };
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(returnedConfig),
      });

      const { api } = await import('@frontend/utils/api');
      const result = await api.updateConfig({
        app: { default_term: { cols: 100, rows: 30, font_size: 16 } },
      });
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/config',
        expect.objectContaining({
          method: 'PUT',
          body: expect.stringContaining('"cols":100'),
        })
      );
      expect(result.app.default_term.cols).toBe(100);
    });
  });

  describe('VNC API methods', () => {
    let fetchSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      fetchSpy = vi.fn();
      vi.stubGlobal('fetch', fetchSpy);
      clearStorage();
    });

    it('getVncSessions calls GET /api/vnc/sessions and returns VncSession[]', async () => {
      const sessions = [
        {
          id: 'v1', kind: 'vnc', owner: 'user', host_id: 'h1', hostname: 'myhost',
          vnc_port: 5900, row: 0, col: 0, state: 'connected', created_at: '', updated_at: '',
          title: 'Desktop', persistent: false,
        },
      ];
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(sessions),
      });

      const { api } = await import('@frontend/utils/api');
      const result = await api.getVncSessions();
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vnc/sessions',
        expect.objectContaining({ headers: expect.any(Object) })
      );
      expect(result).toEqual(sessions);
    });

    it('createVncSession sends POST /api/vnc/sessions with request body and returns VncSession', async () => {
      const session = {
        id: 'v2', kind: 'vnc', owner: 'user', host_id: 'h1', hostname: 'myhost',
        vnc_port: 5900, row: 1, col: 2, state: 'connected', created_at: '', updated_at: '',
        title: 'Desktop', persistent: false,
      };
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 201,
        json: () => Promise.resolve(session),
      });

      const { api } = await import('@frontend/utils/api');
      const result = await api.createVncSession({ hostname: 'myhost', vnc_port: 5900, row: 1, col: 2 });
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vnc/sessions',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"hostname":"myhost"'),
        })
      );
      expect(result.id).toBe('v2');
    });

    it('createVncSession includes vnc_password in body when caller provides it (no stripping)', async () => {
      const session = {
        id: 'v3', kind: 'vnc', owner: 'user', host_id: 'h1', hostname: 'myhost',
        vnc_port: 5901, row: 0, col: 0, state: 'connected', created_at: '', updated_at: '',
        title: 'Desktop', persistent: false,
      };
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 201,
        json: () => Promise.resolve(session),
      });

      const { api } = await import('@frontend/utils/api');
      await api.createVncSession({ hostname: 'myhost', vnc_port: 5901, vnc_password: 'secret' });
      const callBody = fetchSpy.mock.calls[0][1].body as string;
      // The api layer does JSON.stringify(req) directly, so vnc_password passes through
      expect(callBody).toContain('"vnc_password":"secret"');
    });

    it('deleteVncSession sends DELETE /api/vnc/sessions/:id', async () => {
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 204,
        json: () => Promise.reject(),
      });

      const { api } = await import('@frontend/utils/api');
      await api.deleteVncSession('session-id');
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vnc/sessions/session-id',
        expect.objectContaining({ method: 'DELETE' })
      );
    });

    it('reconnectVncSession sends POST /api/vnc/sessions/:id/reconnect', async () => {
      const session = {
        id: 'session-id', kind: 'vnc', owner: 'user', host_id: 'h1', hostname: 'myhost',
        vnc_port: 5900, row: 0, col: 0, state: 'connected', created_at: '', updated_at: '',
        title: 'Desktop', persistent: false,
      };
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(session),
      });

      const { api } = await import('@frontend/utils/api');
      await api.reconnectVncSession('session-id');
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vnc/sessions/session-id/reconnect',
        expect.objectContaining({ method: 'POST' })
      );
    });

    it('moveVncSession sends PATCH /api/vnc/sessions/:id with { row, col }', async () => {
      const session = {
        id: 'session-id', kind: 'vnc', owner: 'user', host_id: 'h1', hostname: 'myhost',
        vnc_port: 5900, row: 2, col: 3, state: 'connected', created_at: '', updated_at: '',
        title: 'Desktop', persistent: false,
      };
      fetchSpy.mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(session),
      });

      const { api } = await import('@frontend/utils/api');
      await api.moveVncSession('session-id', 2, 3);
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vnc/sessions/session-id',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ row: 2, col: 3 }),
        })
      );
    });
  });

  describe('buildVncWsUrl', () => {
    it('builds ws:// URL for http origin with correct path', async () => {
      const { buildVncWsUrl } = await import('@frontend/utils/api');
      const url = buildVncWsUrl('session-id');
      expect(url).toContain('ws://');
      expect(url).toContain('/api/vnc/ws/session-id');
    });

    it('omits token query when no token is set', async () => {
      const { buildVncWsUrl } = await import('@frontend/utils/api');
      const url = buildVncWsUrl('session-id');
      expect(url).not.toContain('?token=');
    });

    it('includes token in query when token is present', async () => {
      localStorage.setItem('webmux_token', 'thetoken');
      const { buildVncWsUrl } = await import('@frontend/utils/api');
      const url = buildVncWsUrl('session-id');
      expect(url).toContain('?token=thetoken');
      clearStorage();
    });

    it('builds wss:// URL when page is served over https', async () => {
      const originalDescriptor = Object.getOwnPropertyDescriptor(window, 'location');
      Object.defineProperty(window, 'location', {
        value: { ...window.location, protocol: 'https:', host: 'localhost' },
        configurable: true,
      });

      const { buildVncWsUrl } = await import('@frontend/utils/api');
      const url = buildVncWsUrl('session-id');
      expect(url).toContain('wss://');
      expect(url).toContain('/api/vnc/ws/session-id');

      if (originalDescriptor) {
        Object.defineProperty(window, 'location', originalDescriptor);
      }
    });
  });
});
