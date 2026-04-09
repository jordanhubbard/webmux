export interface AppConfig {
  app: {
    name: string;
    listen_host: string;
    http_port: number;
    https_port: number;
    secure_mode: boolean;
    trusted_http_allowed: boolean;
    default_term: {
      cols: number;
      rows: number;
      font_size: number;
    };
    transport: {
      prefer_mosh: boolean;
      ssh_fallback: boolean;
      mosh_server_path: string;
    };
    guacd?: {
      host: string;
      port: number;
    };
    // Populated from WEBMUX_EXEC_COMMAND env var at runtime; not persisted to app.yaml.
    exec_command?: string;
  };
}

export interface AuthUser {
  username: string;
  password_hash: string;
}

export interface AuthConfig {
  auth: {
    mode: 'none' | 'local';
    users: AuthUser[];
  };
}

export type TransportType = 'ssh' | 'mosh' | 'exec';
export type SessionKind = 'terminal' | 'vnc' | 'rdp';
export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

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

export interface HostsConfig {
  hosts: HostEntry[];
}

export interface TileLayout {
  session_id: string;
  row: number;
  col: number;
}

export interface LayoutConfig {
  layout: {
    font_size: number;
    tiles: TileLayout[];
  };
}

export interface KeyEntry {
  id: string;
  type: string;
  private_key_path: string;
  encrypted: boolean;
  description: string;
}

export interface KeysConfig {
  keys: KeyEntry[];
}

export interface Session {
  id: string;
  kind: 'terminal';
  owner: string;
  transport: TransportType;
  host_id: string;
  hostname: string;
  port: number;
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

export interface Viewer {
  id: string;
  session_id: string;
  has_focus: boolean;
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

// Session template — pre-configured session setup (e.g. "claude-cli")
export interface SessionTemplate {
  id:           string;
  name:         string;
  description:  string;
  icon:         string;
  initialCmd?:  string;  // command to run after connection is established
  setupSteps:   string[];  // human-readable setup guide steps
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
  // Falls back to WEBMUX_EXEC_COMMAND env var if not set.
  exec_command?: string;
  cols?: number;
  rows?: number;
  row?: number;
  col?: number;
  // Optional: session template ID (e.g. "claude-cli")
  template_id?: string;
  // Optional: command to run immediately after connection (injected into PTY)
  initial_cmd?: string;
}

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

