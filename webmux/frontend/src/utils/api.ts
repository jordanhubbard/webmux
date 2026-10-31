import type { CoordinationDrainResult, CoordinationSendResult, CoordinationStatus, WorkspaceListing } from '../components/CoordinationDialog';
import type {
  AgentTmuxSession,
  AppConfig,
  AuthMe,
  AuthStatus,
  AuthUserInfo,
  CreateRdpSessionRequest,
  CreateSessionRequest,
  CreateVncSessionRequest,
  DeepPartial,
  HostEntry,
  ManagedClient,
  KeyEntry,
  AgentsConfig,
  RdpSession,
  Session,
  VncSession,
} from '../types';
import { signalAuthExpired } from './authSession';

const API_BASE = '/api';

function getToken(): string | null {
  return localStorage.getItem('webmux_token');
}

function authHeaders(): HeadersInit {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers: {
        ...authHeaders(),
        ...(options.headers || {}),
      },
    });
  } catch {
    throw new Error('Cannot reach server');
  }

  if (!res.ok) {
    if (res.status === 401 && getToken()) signalAuthExpired();
    const err = await res.json().catch(() => ({ error: `${res.status} ${res.statusText}` }));
    throw new Error((err as { error?: string }).error || `${res.status} ${res.statusText}`);
  }

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export interface BrowserAction {
  action: string; url?: string; text?: string; key?: string; code?: string;
  keyCode?: number; modifiers?: number; x?: number; y?: number; deltaY?: number; target?: string;
}
export interface BrowserFrame {
  image?: string; url: string; target: string; tabs: { id: string; title: string }[]; error?: string;
}

export interface PermissionResult {
  state: string; detail?: string; checked_at?: string;
  folders?: { name: string; state: string }[];
}
export interface MacSetup {
  skipped: boolean;
  status: {
    instance: string; supported: boolean; hostname: string; executable: string; permission_target: string;
    can_open_settings: boolean; local: PermissionResult; ssh: PermissionResult;
  };
}

export interface ConnectedServer { id: string; name: string }

export const api = {
  getServers: () => request<{ version: number; servers: ConnectedServer[] }>('/servers'),
  getServerSessions: (server: string) => request<Session[]>(`/servers/${encodeURIComponent(server)}/sessions`),
  createServerSession: (server: string) => request<Session>(`/servers/${encodeURIComponent(server)}/sessions`, { method: 'POST', body: JSON.stringify({ transport: 'local' }) }),
  reconnectServerSession: (server: string, id: string) => request<Session>(`/servers/${encodeURIComponent(server)}/sessions/${encodeURIComponent(id)}/reconnect`, { method: 'POST', body: '{}' }),
  sendServerMessage: (server: string, id: string, message: string) => request<{ queued: boolean; processed: boolean }>(`/servers/${encodeURIComponent(server)}/sessions/${encodeURIComponent(id)}/messages`, { method: 'POST', body: JSON.stringify({ message }) }),
  getCoordination: () => request<CoordinationStatus>('/coordination'),
  createCoordinatedAgent: (input: { provider: string; mode: 'new' | 'existing'; projectId?: string; projectName: string; context?: string; name: string; host: string; workspace: string; runtime: string; row?: number; col?: number }) => request<{ target: string; session: Session }>('/coordination/agents', { method: 'POST', body: JSON.stringify(input) }),
  browseCoordinationWorkspaces: (provider: string, host: string, path?: string) => request<WorkspaceListing>('/coordination/workspaces', { method: 'POST', body: JSON.stringify({ provider, host, path }) }),
  attachCoordinatedAgent: (provider: string, target: string, position?: { row: number; col: number }) => request<Session>('/coordination/attach', { method: 'POST', body: JSON.stringify({ provider, target, ...position }) }),
  sendCoordinationMessage: (provider: string, target: string, message: string, queue: boolean) => request<CoordinationSendResult>('/coordination/messages', { method: 'POST', body: JSON.stringify({ provider, target, message, queue }) }),
  getCoordinationSendStatus: (provider: string, target: string, sendId: string) => request<Record<string, unknown>>('/coordination/send-status', { method: 'POST', body: JSON.stringify({ provider, target, sendId }) }),
  drainCoordinationRemote: (provider: string, remote: string, target: string) => request<CoordinationDrainResult>('/coordination/drain', { method: 'POST', body: JSON.stringify({ provider, remote, target }) }),
  getOpenRig: () => request<{ available: boolean; message: string; session: Session | null; views?: Record<string, Session>; proof?: { phase: string; run_id: string; message: string; nonce_a?: string; nonce_b?: string; digest?: string } | null }>('/openrig'),
  getOpenRigAgents: () => request<{ target: string; runtime: string; rig: string; status: string }[]>('/openrig/agents'),
  attachOpenRig: (view = 'dashboard', target?: string) => request<Session>('/openrig/attach', { method: 'POST', body: JSON.stringify({ view, target }) }),
  stopOpenRigProof: () => request('/openrig/proof/stop', { method: 'POST' }),
  getMacSetup: () => request<MacSetup>('/setup/macos'),
  macSetupAction: (action: string, transport?: 'local' | 'ssh') => request<PermissionResult | { ok?: boolean; restarting?: boolean }>('/setup/macos', { method: 'POST', body: JSON.stringify({ action, transport }) }),
  beginAuthHandoff: (id: string, authorization: string) => request<{ id: string; expires_in: number }>(`/sessions/${id}/auth-handoff`, { method: 'POST', body: JSON.stringify({ authorization }) }),
  deliverAuthHandoff: (id: string, ticket: string, callback: string) => request<{ delivered: boolean }>(`/sessions/${id}/auth-handoff`, { method: 'POST', body: JSON.stringify({ id: ticket, callback }) }),
  endAuthHandoff: (id: string, ticket: string) => request<void>(`/sessions/${id}/auth-handoff`, { method: 'DELETE', body: JSON.stringify({ id: ticket }) }),
  browserAction: (id: string, action: BrowserAction) => request<BrowserFrame>(`/sessions/${id}/browser`, { method: 'POST', body: JSON.stringify(action) }),
  endBrowser: (id: string) => request<void>(`/sessions/${id}/browser`, { method: 'DELETE' }),
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
  refreshToken: () => request<{ token: string; mode: string }>('/auth/refresh', { method: 'POST' }),
  register: (username: string, password: string, admin = false) =>
    request<{ username: string; admin: boolean }>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password, admin }),
    }),
  getMe: () => request<AuthMe>('/auth/me'),
  getUsers: () => request<AuthUserInfo[]>('/auth/users'),
  deleteUser: (username: string) =>
    request<void>(`/auth/users/${encodeURIComponent(username)}`, { method: 'DELETE' }),

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
  setSessionProject: (id: string, project_label: string) =>
    request<Session>(`/sessions/${id}`, { method: 'PATCH', body: JSON.stringify({ project_label }) }),
  setMinimized: (id: string, minimized: boolean) =>
    request<Session>(`/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ minimized }),
    }),
  uploadTerminalFile: async (id: string, file: File, signal?: AbortSignal): Promise<{ path: string; name: string; size: number }> => {
    const headers: Record<string, string> = { 'Content-Type': 'application/octet-stream', 'X-Filename': encodeURIComponent(file.name) };
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
    const response = await fetch(`${API_BASE}/sessions/${encodeURIComponent(id)}/files`, { method: 'POST', headers, body: file, signal });
    if (!response.ok) {
      if (response.status === 401 && token) signalAuthExpired();
      const error = await response.json().catch(() => ({ error: `Upload failed (${response.status})` })) as { error?: string };
      throw new Error(error.error || 'Upload failed');
    }
    return response.json();
  },
  // Upload
  uploadFile: async (file: File): Promise<{ path: string; name: string; size: number }> => {
    const token = getToken();
    const headers: HeadersInit = {
      'Content-Type': 'application/octet-stream',
      'X-Filename': file.name,
    };
    if (token) headers['Authorization'] = `Bearer ${token}`;
    const res = await fetch(`${API_BASE}/upload`, {
      method: 'POST',
      headers,
      body: file,
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error((err as { error?: string }).error || res.statusText);
    }
    return res.json();
  },
  // VNC Sessions
  getVncSessions: () => request<VncSession[]>('/vnc/sessions'),
  createVncSession: (req: CreateVncSessionRequest) =>
    request<VncSession>('/vnc/sessions', { method: 'POST', body: JSON.stringify(req) }),
  deleteVncSession: (id: string) =>
    request<void>(`/vnc/sessions/${id}`, { method: 'DELETE' }),
  reconnectVncSession: (id: string) =>
    request<VncSession>(`/vnc/sessions/${id}/reconnect`, { method: 'POST' }),
  moveVncSession: (id: string, row: number, col: number) =>
    request<VncSession>(`/vnc/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ row, col }),
    }),

  // RDP Sessions
  getRdpSessions: () => request<RdpSession[]>('/rdp/sessions'),
  createRdpSession: (req: CreateRdpSessionRequest) =>
    request<RdpSession>('/rdp/sessions', { method: 'POST', body: JSON.stringify(req) }),
  deleteRdpSession: (id: string) =>
    request<void>(`/rdp/sessions/${id}`, { method: 'DELETE' }),
  reconnectRdpSession: (id: string) =>
    request<RdpSession>(`/rdp/sessions/${id}/reconnect`, { method: 'POST' }),
  moveRdpSession: (id: string, row: number, col: number) =>
    request<RdpSession>(`/rdp/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ row, col }),
    }),

  // Hosts
  getHosts: () => request<HostEntry[]>('/hosts'),
  getClients: () => request<ManagedClient[]>('/clients'),
  bindClient: (hostId: string, pairCode: string) => request<{ client_id: string; host_id: string }>(`/hosts/${encodeURIComponent(hostId)}/client`, { method: 'POST', body: JSON.stringify({ pair_code: pairCode }) }),
  unbindClient: (hostId: string) => request<void>(`/hosts/${encodeURIComponent(hostId)}/client`, { method: 'DELETE' }),
  revokeClient: (clientId: string) => request<void>(`/clients/${encodeURIComponent(clientId)}`, { method: 'DELETE' }),
  refreshClient: (clientId: string) => request<ManagedClient['snapshot']>(`/clients/${encodeURIComponent(clientId)}/refresh`, { method: 'POST', body: '{}' }),
  associateSessionHost: (sessionId: string, hostId: string) => request<Session>(`/sessions/${encodeURIComponent(sessionId)}`, { method: 'PATCH', body: JSON.stringify({ host_id: hostId }) }),
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

  // Agent sessions
  getAgentConfig: () => request<AgentsConfig>('/agents/config'),
  getAllAgentSessions: () => request<AgentTmuxSession[]>('/agents/sessions'),
  getAgentSessions: (agentId: string) => request<AgentTmuxSession[]>(`/agents/${agentId}/sessions`),
  attachAgentSession: (agentId: string, req: { name: string; cols: number; rows: number }) =>
    request<Session>(`/agents/${agentId}/attach`, { method: 'POST', body: JSON.stringify(req) }),
  createAgentScratch: (agentId: string, req: { selectedName?: string; cols: number; rows: number }) =>
    request<Session>(`/agents/${agentId}/scratch`, { method: 'POST', body: JSON.stringify(req) }),
};

export function buildWsUrl(sessionId: string): string {
  const token = getToken();
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  const remote = /^@([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)$/.exec(sessionId);
  const endpoint = remote ? `/api/servers/${remote[1]}/term/${remote[2]}` : `/api/term/${encodeURIComponent(sessionId)}`;
  return `${proto}//${host}${endpoint}${query}`;
}

export function buildVncWsUrl(sessionId: string): string {
  const token = getToken();
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  return `${proto}//${host}/api/vnc/ws/${sessionId}${query}`;
}

export function buildRdpWsUrl(sessionId: string): string {
  const token = getToken();
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  return `${proto}//${host}/api/rdp/ws/${sessionId}${query}`;
}
