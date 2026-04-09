export type TransportType = 'ssh' | 'mosh' | 'exec';
export type SessionKind = 'terminal' | 'vnc' | 'rdp';
export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

export interface Session {
  id: string;
  kind: 'terminal';
  owner: string;
  transport: TransportType;
  host_id: string;
  hostname: string;
  username: string;
  key_id: string;
  exec_command?: string;
  cols: number;
  rows: number;
  row: number;
  col: number;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
  title: string;
  persistent: boolean;
  minimized: boolean;
}

export interface HostEntry {
  id: string;
  hostname: string;
  port: number;
  username: string;
  transport: TransportType;
  key_id: string;
  tags: string[];
  mosh_allowed: boolean;
  vnc_enabled: boolean;
  vnc_port: number;
  rdp_enabled: boolean;
  rdp_port: number;
}

export interface AuthStatus {
  mode: 'none' | 'local';
  bootstrap_required: boolean;
}

export interface WebSocketMessage {
  type: 'input' | 'resize' | 'output' | 'status' | 'focus' | 'viewer_join' | 'viewer_leave' | 'error';
  session_id?: string;
  data?: string;
  cols?: number;
  rows?: number;
  state?: ConnectionState;
  viewer_id?: string;
  viewer_count?: number;
  focus_owner?: string;
  message?: string;
}

export interface CreateSessionRequest {
  host_id?: string;
  hostname?: string;
  port?: number;
  username: string;
  password?: string;
  key_id?: string;
  transport?: TransportType;
  // For exec transport: command template with {host}, {port}, {user} substitutions.
  // Falls back to WEBMUX_EXEC_COMMAND env var on the server if not set.
  exec_command?: string;
  cols?: number;
  rows?: number;
  row?: number;
  col?: number;
}

export interface KeyEntry {
  id: string;
  type: string;
  private_key_path: string;
  encrypted: boolean;
  description: string;
}

export interface AppConfig {
  app: {
    name: string;
    http_port: number;
    https_port: number;
    secure_mode: boolean;
    trusted_http_allowed: boolean;
    default_term: {
      cols: number;
      rows: number;
      font_size: number;
    };
    // Set by WEBMUX_EXEC_COMMAND on the server; drives exec-transport defaults in the UI.
    exec_command?: string;
  };
}

export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

export interface VncSession {
  id: string;
  kind: 'vnc';
  owner: string;
  host_id: string;
  hostname: string;
  vnc_port: number;
  row: number;
  col: number;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
  title: string;
  persistent: boolean;
}

export interface CreateVncSessionRequest {
  host_id?: string;
  hostname?: string;
  vnc_port?: number;
  vnc_password?: string;
  row?: number;
  col?: number;
}

export interface RdpSession {
  id: string;
  kind: 'rdp';
  owner: string;
  host_id: string;
  hostname: string;
  rdp_port: number;
  rdp_username: string;
  rdp_domain: string;
  row: number;
  col: number;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
  title: string;
  persistent: boolean;
}

export interface CreateRdpSessionRequest {
  host_id?: string;
  hostname?: string;
  rdp_port?: number;
  rdp_username?: string;
  rdp_password?: string;
  rdp_domain?: string;
  row?: number;
  col?: number;
}
