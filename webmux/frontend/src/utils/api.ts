import type { CoordinationDrainResult, CoordinationSendResult, CoordinationStatus, WorkspaceListing } from '../components/CoordinationDialog';
import type { RigStopPreview, RigStopReceipt } from '../components/RigStopDialog';
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
  SessionRecovery,
  RecoveryRequest,
  VncSession,
} from '../types';
import { signalAuthExpired } from './authSession';

const API_BASE = '/api';

class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly body: unknown) { super(message); }
}
export class CoordinationCreationUnconfirmedError extends Error {
  constructor(message: string) { super(message); }
}
export class CoordinationTrustRequiredError extends Error {
  constructor(readonly requestId: string) { super('Review project trust in native Codex before retrying the saved request.'); }
}

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
    throw new ApiError((err as { error?: string }).error || `${res.status} ${res.statusText}`, res.status, err);
  }

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export interface CoordinationSendInput {
  requestId: string; scope: string; provider: string; target: string; message: string; queue: boolean;
}
export function checkedCoordinationSendOutcome(value: CoordinationSendResult, id: string, scope: string): CoordinationSendResult {
  if (!value || value.requestId !== id || !['unobserved', 'pending', 'uncertain', 'completed', 'refused', 'cancelled'].includes(value.requestState || '')
    || (value.request && (value.request.requestId !== id || value.request.scope !== scope || value.request.provider !== 'agent-deck' || !value.request.target || typeof value.request.message !== 'string' || typeof value.request.queue !== 'boolean'))) {
    throw new Error('The original send outcome is unconfirmed. Keep its saved request; do not resend as a new instruction.');
  }
  return value;
}

export interface BrowserAction {
  action: string; profileId?: string; force?: boolean; url?: string; text?: string; key?: string; code?: string;
  keyCode?: number; modifiers?: number; x?: number; y?: number; deltaY?: number; deltaX?: number; target?: string;
  width?: number; height?: number; button?: string; buttons?: number; clickCount?: number;
  backward?: boolean; ignoreCache?: boolean; accept?: boolean; element?: string; files?: { name: string; data: string }[];
}
export interface BrowserPageText { nodes: { role: string; name: string; depth: number; element?: string; canFocus?: boolean; canActivate?: boolean }[]; truncated: boolean }
export interface BrowserFrame {
  pageText?: BrowserPageText;
  protocol?: number;
  control?: { owned: boolean; available: boolean; expiresAt?: string; token?: string };
  profileId?: string; profileName?: string;
  image?: string; url: string; target: string; tabs: { id: string; title: string }[]; error?: string;
  width?: number; height?: number; canBack?: boolean; canForward?: boolean;
  dialog?: { type: string; message: string; prompt: string };
  select?: { options: { value: string; label: string; selected: boolean; disabled: boolean }[] };
  fileChooser?: { multiple: boolean };
  downloads?: { id: string; name: string; state: string; bytes: number }[];
  file?: { name: string; data: string };
  clipboard?: string; found?: boolean;
}

export interface BrowserProfile { id: string; name: string }
let browserViewer = '';
const browserTokens = new Map<string, string>();
function browserIdentity(id: string) {
  if (!browserViewer) browserViewer = Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('');
  return { viewer: browserViewer, controlToken: browserTokens.get(id) ?? '' };
}
async function browserAction(id: string, action: BrowserAction) {
  const result = await request<BrowserFrame>(`/sessions/${id}/browser`, { method: 'POST', body: JSON.stringify({ ...action, ...browserIdentity(id) }) });
  if (result.control?.owned && result.control.token) browserTokens.set(id, result.control.token);
  else browserTokens.delete(id);
  return result;
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

export interface ManagedServer extends ConnectedServer { url: string; allow_http: boolean; token_env: string; credential_ready: boolean }
export interface ServerGrant { id: string; owner: string; token_env: string; credential_ready: boolean }
export interface ServerDirectory { servers: ManagedServer[]; grants: ServerGrant[] }
export interface ServerCredentialInput { token_env: string; credential: string }

export interface USBForwardingSpec { host: string; ssh_port: number; export_port: number; seconds: number }
export interface USBForwardingLease {
  id: string; spec: USBForwardingSpec;
  phase: 'pending' | 'starting' | 'ready' | 'stopping' | 'ended' | 'failed' | 'uncertain' | 'expired';
  created_at: string; expires_at: string; last_seen: string; remote_port: number;
  stop_requested: boolean; release_confirmed: boolean; stale: boolean;
}
export interface USBForwardingTicket { lease: USBForwardingLease; pair_code: string }

export interface CoordinationCreateRequest {
  provider: string; mode: 'new' | 'existing'; projectId?: string; projectName: string; context?: string;
  name: string; host: string; workspace: string; runtime: string; row?: number; col?: number; inspect?: boolean; trustRequest?: string; creationRequestId?: string; continueOriginal?: boolean;
}
export type CoordinationCreateResponse = { target: string; session: Session; creation?: never }
  | { creation: { state: 'uncertain'; stage: 'project' | 'agent'; code: string; operationId: string }; session?: never; target?: never };

async function createCoordinatedAgent(input: CoordinationCreateRequest): Promise<CoordinationCreateResponse> {
  try {
    const endpoint = input.creationRequestId
      ? '/coordination/creation-requests/' + encodeURIComponent(input.creationRequestId) : '/coordination/agents';
    const result = await request<CoordinationCreateResponse>(endpoint, { method: 'POST', body: JSON.stringify(input) });
    if (input.creationRequestId) {
      const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
      if (!object(result)) throw new Error('Creation response was incomplete.');
      const creation = result.creation;
      const pending = object(creation) && Object.keys(result).join(',') === 'creation'
        && Object.keys(creation).sort().join(',') === 'code,operationId,stage,state'
        && creation.state === 'uncertain' && ['project', 'agent'].includes(String(creation.stage))
        && typeof creation.code === 'string' && /^[a-z][a-z0-9_]{0,95}$/.test(creation.code)
        && typeof creation.operationId === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(creation.operationId);
      const attached = !('creation' in result) && !('blocked' in result) && !('refused' in result)
        && typeof result.target === 'string' && !!result.target && object(result.session)
        && typeof result.session.id === 'string' && !!result.session.id;
      if (!pending && !attached) throw new Error('Creation response was incomplete.');
    }
    return result;
  }
  catch (error) {
    if (error instanceof ApiError && error.status === 409 && input.provider === 'rig' && input.runtime === 'codex'
      && (!input.host || input.host === 'localhost') && (input.mode === 'new' || input.mode === 'existing')
      && error.body && typeof error.body === 'object' && 'blocked' in error.body
      && !('creation' in error.body) && !('session' in error.body)) {
      const blocked = error.body.blocked;
      if (blocked && typeof blocked === 'object' && Object.keys(blocked).sort().join(',') === 'code,requestId,state'
        && 'state' in blocked && blocked.state === 'blocked'
        && 'code' in blocked && blocked.code === 'native_project_trust_required' && 'requestId' in blocked
        && typeof blocked.requestId === 'string' && /^[a-f0-9]{64}$/.test(blocked.requestId)) {
        throw new CoordinationTrustRequiredError(blocked.requestId);
      }
    }
    if (input.creationRequestId) {
      // Only an explicit pre-dispatch refusal can clear the first attempt.
      // A refusal of a continuation says nothing about the ORIGINAL outcome.
      const refused = error instanceof ApiError && error.body && typeof error.body === 'object'
        && !('creation' in error.body) && !('session' in error.body) && !('blocked' in error.body)
        && 'refused' in error.body ? error.body.refused : null;
      if (error instanceof ApiError && [400, 409].includes(error.status) && !input.continueOriginal && !input.inspect && !input.trustRequest && refused && typeof refused === 'object'
        && Object.keys(refused).sort().join(',') === 'code,state' && 'state' in refused && refused.state === 'refused'
        && 'code' in refused && refused.code === 'invalid_creation_request') throw error;
      throw new CoordinationCreationUnconfirmedError(error instanceof Error ? error.message : 'Creation response was not confirmed.');
    }
    throw error;
  }
}

export const api = {
  getUSBForwardings: (signal?: AbortSignal) => request<USBForwardingLease[]>('/usb-forwardings', { signal, cache: 'no-store' }),
  createUSBForwarding: (spec: USBForwardingSpec) => request<USBForwardingTicket>('/usb-forwardings', { method: 'POST', body: JSON.stringify(spec) }),
  stopUSBForwarding: (id: string) => request<USBForwardingLease>(`/usb-forwardings/${encodeURIComponent(id)}/stop`, { method: 'POST' }),

  getSessionRecovery: (id: string, signal?: AbortSignal) => request<SessionRecovery>(`/session-recovery/${encodeURIComponent(id)}`, { signal, cache: 'no-store' }),
  recoverSession: (id: string, input: RecoveryRequest) => request<{ outcome: string; code?: string }>(`/session-recovery/${encodeURIComponent(id)}`, { method: 'POST', body: JSON.stringify(input) }),
  setAutoRecovery: (id: string, enabled: boolean) => request<{ auto_resume: boolean }>(`/session-recovery/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ auto_resume: enabled }) }),
  getServerDirectory: () => request<ServerDirectory>('/server-directory'),
  saveServer: (id: string, input: ServerCredentialInput & { name: string; url: string; allow_http: boolean }) => request<void>(`/servers/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteServer: (id: string) => request<void>(`/servers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  saveServerGrant: (id: string, input: ServerCredentialInput & { owner: string }) => request<void>(`/server-grants/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteServerGrant: (id: string) => request<void>(`/server-grants/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  getServers: () => request<{ version: number; servers: ConnectedServer[] }>('/servers'),
  getServerSessions: (server: string) => request<Session[]>(`/servers/${encodeURIComponent(server)}/sessions`),
  createServerSession: (server: string) => request<Session>(`/servers/${encodeURIComponent(server)}/sessions`, { method: 'POST', body: JSON.stringify({ transport: 'local' }) }),
  reconnectServerSession: (server: string, id: string) => request<Session>(`/servers/${encodeURIComponent(server)}/sessions/${encodeURIComponent(id)}/reconnect`, { method: 'POST', body: '{}' }),
  sendServerMessage: (server: string, id: string, message: string) => request<{ queued: boolean; processed: boolean }>(`/servers/${encodeURIComponent(server)}/sessions/${encodeURIComponent(id)}/messages`, { method: 'POST', body: JSON.stringify({ message }) }),
  getCoordination: (provider?: string, signal?: AbortSignal) => request<CoordinationStatus>(`/coordination${provider ? `?provider=${encodeURIComponent(provider)}` : ''}`, signal ? { signal } : undefined),
  previewRigStop: (target: string, reason: string) => request<RigStopPreview>('/coordination/stop/preview', { method: 'POST', body: JSON.stringify({ provider: 'rig', target, reason }) }),
  executeRigStop: (id: string) => request<RigStopReceipt>('/coordination/stop', { method: 'POST', body: JSON.stringify({ id, confirm: true }) }),
  getRigChanges: (signal?: AbortSignal) => request<{ contract: string; available: boolean; changed: boolean }>('/coordination/changes', { signal }),
  getRigObservations: (signal?: AbortSignal) => request<CoordinationStatus>('/coordination?provider=rig&observations=rig', { signal }),
  createCoordinatedAgent,
  getRigProjectPods: (projectId: string, projectName: string) => request<{ projectId: string; projectName: string; pods: string[]; source: 'native-spec' | 'legacy-seats' }>('/coordination/rig/pods', { method: 'POST', body: JSON.stringify({ projectId, projectName }) }),
  browseCoordinationWorkspaces: (provider: string, host: string, path?: string) => request<WorkspaceListing>('/coordination/workspaces', { method: 'POST', body: JSON.stringify({ provider, host, path }) }),
  attachCoordinatedAgent: (provider: string, target: string, position?: { row: number; col: number }) => request<Session>('/coordination/attach', { method: 'POST', body: JSON.stringify({ provider, target, ...position }) }),
  sendCoordinationMessage: async (input: CoordinationSendInput) => checkedCoordinationSendOutcome(await request<CoordinationSendResult>('/coordination/messages', { method: 'POST', body: JSON.stringify(input) }), input.requestId, input.scope),
  getCoordinationSendScope: () => request<{ scope: string }>('/coordination/send-intents', { cache: 'no-store' }),
  inspectCoordinationSend: async (id: string, scope: string) => checkedCoordinationSendOutcome(await request<CoordinationSendResult>(`/coordination/send-intents/${encodeURIComponent(id)}`, { cache: 'no-store', headers: { 'X-WebMux-Send-Scope': scope } }), id, scope),
  dismissCoordinationSend: async (id: string, scope: string) => checkedCoordinationSendOutcome(await request<CoordinationSendResult>(`/coordination/send-intents/${encodeURIComponent(id)}/dismiss`, { method: 'POST', headers: { 'X-WebMux-Send-Scope': scope } }), id, scope),
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
  browserAction,
  endBrowser: async (id: string) => {
    await request<void>(`/sessions/${id}/browser`, { method: 'DELETE', body: JSON.stringify(browserIdentity(id)) });
    browserTokens.delete(id);
  },
  releaseBrowser: async (id: string) => {
    if (!browserTokens.has(id)) return;
    const identity = browserIdentity(id); browserTokens.delete(id);
    await request<BrowserFrame>(`/sessions/${id}/browser`, { method: 'POST', body: JSON.stringify({ action: 'release', ...identity }) });
  },
  getBrowserProfiles: (id: string) => request<BrowserProfile[]>(`/sessions/${id}/browser/profiles`),
  createBrowserProfile: (id: string, name: string) => request<BrowserProfile>(`/sessions/${id}/browser/profiles`, { method: 'POST', body: JSON.stringify({ name }) }),
  deleteBrowserProfile: (id: string, profileId: string) => request<void>(`/sessions/${id}/browser/profiles`, { method: 'DELETE', body: JSON.stringify({ profileId }) }),
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
  getSession: (id: string) => request<Session>(`/sessions/${id}`),
  stopLocalSession: (id: string, action: { id: string; selection: string; mode: 'stop' | 'inspect' | 'continue'; confirm: boolean }) =>
    request<{ outcome: 'stopped' | 'uncertain'; message?: string; session: Session }>(`/sessions/${id}/local-stop`, { method: 'POST', body: JSON.stringify(action) }),
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
