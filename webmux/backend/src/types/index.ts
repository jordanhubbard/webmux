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
    };
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

export type TransportType = 'ssh' | 'mosh';
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
  owner: string;
  transport: TransportType;
  host_id: string;
  hostname: string;
  port: number;
  username: string;
  key_id: string;
  cols: number;
  rows: number;
  row: number;
  col: number;
  state: ConnectionState;
  created_at: string;
  updated_at: string;
  title: string;
  persistent: boolean;
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

export interface CreateSessionRequest {
  host_id?: string;
  hostname?: string;
  port?: number;
  username: string;
  password?: string;
  key_id?: string;
  transport?: TransportType;
  cols?: number;
  rows?: number;
  row?: number;
  col?: number;
}

