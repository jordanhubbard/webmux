import type { Session, HostEntry, KeyEntry, AuthStatus, AppConfig, DeepPartial, CreateSessionRequest } from '../types';

const API_BASE = '/api';

function getToken(): string | null {
  return localStorage.getItem('webmux_token');
}

function authHeaders(): HeadersInit {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      ...authHeaders(),
      ...(options.headers || {}),
    },
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error((err as { error?: string }).error || res.statusText);
  }

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  // Auth
  getAuthStatus: () => request<AuthStatus>('/auth/status'),
  login: (username: string, password: string) =>
    request<{ token: string; mode: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  bootstrap: (username: string, password: string) =>
    request<{ token: string; mode: string }>('/auth/bootstrap', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  register: (username: string, password: string) =>
    request<{ username: string }>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  // Sessions
  getSessions: () => request<Session[]>('/sessions'),
  createSession: (req: CreateSessionRequest) =>
    request<Session>('/sessions', { method: 'POST', body: JSON.stringify(req) }),
  deleteSession: (id: string) =>
    request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  reconnectSession: (id: string, password?: string) =>
    request<Session>(`/sessions/${id}/reconnect`, {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  moveSession: (id: string, row: number, col: number) =>
    request<Session>(`/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ row, col }),
    }),
  renameSession: (id: string, title: string) =>
    request<Session>(`/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    }),
  // Hosts
  getHosts: () => request<HostEntry[]>('/hosts'),
  createHost: (host: Partial<HostEntry>) =>
    request<HostEntry>('/hosts', { method: 'POST', body: JSON.stringify(host) }),
  deleteHost: (id: string) =>
    request<void>(`/hosts/${id}`, { method: 'DELETE' }),

  // Keys
  getKeys: () => request<Pick<KeyEntry, 'id' | 'type' | 'encrypted' | 'description'>[]>('/keys'),

  // Config
  getConfig: () => request<AppConfig>('/config'),
  updateConfig: (config: DeepPartial<AppConfig>) =>
    request<AppConfig>('/config', { method: 'PUT', body: JSON.stringify(config) }),
};

export function buildWsUrl(sessionId: string): string {
  const token = getToken();
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  return `${proto}//${host}/api/term/${sessionId}${query}`;
}
