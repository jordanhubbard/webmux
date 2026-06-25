import * as path from 'path';
import type {
  AgentDefinition,
  AgentDefinitionConfig,
  AppConfig,
  HostSwitcherConfig,
  NormalizedAgentsConfig,
  WorkspaceName,
} from '../types';

const AGENT_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/;
const TMUX_SOCKET_RE = /^[a-zA-Z0-9_.-]+$/;

function stringOrDefault(value: unknown, fallback: string): string {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function booleanOrDefault(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

function normalizeWorkspace(value: unknown, id: string): WorkspaceName {
  const workspace = stringOrDefault(value, `agent-${id}`);
  if (workspace.includes('\0')) {
    throw new Error(`Invalid workspace for agent '${id}'`);
  }
  return workspace;
}

function normalizeTmuxSocket(value: unknown, id: string): string {
  const socket = stringOrDefault(value, '');
  if (!socket) {
    throw new Error(`Agent '${id}' requires tmux_socket`);
  }
  if (socket.includes('\0')) {
    throw new Error(`Invalid tmux_socket for agent '${id}'`);
  }
  if (path.isAbsolute(socket)) return socket;
  if (!TMUX_SOCKET_RE.test(socket)) {
    throw new Error(`Invalid tmux_socket for agent '${id}'`);
  }
  return socket;
}

export function normalizeAgentDefinition(raw: AgentDefinitionConfig): AgentDefinition {
  const id = stringOrDefault(raw.id, '');
  if (!AGENT_ID_RE.test(id)) {
    throw new Error(`Invalid agent id '${raw.id}'`);
  }
  const label = stringOrDefault(raw.label, id);
  const pluralLabel = stringOrDefault(raw.plural_label, `${label} Sessions`);
  const badge = stringOrDefault(raw.badge, id.toUpperCase()).slice(0, 16);
  return {
    id,
    label,
    plural_label: pluralLabel,
    badge,
    tmux_socket: normalizeTmuxSocket(raw.tmux_socket, id),
    workspace: normalizeWorkspace(raw.workspace, id),
    enabled: raw.enabled !== false,
  };
}

export function normalizeAgentsConfig(config: AppConfig): NormalizedAgentsConfig {
  const raw = config.app.agents ?? {};
  const enabled = booleanOrDefault(raw.enabled, false);
  const definitions = (enabled ? raw.definitions ?? [] : [])
    .filter(definition => definition.enabled !== false)
    .map(normalizeAgentDefinition)
    .filter(definition => definition.enabled);
  const seen = new Set<string>();
  for (const definition of definitions) {
    if (seen.has(definition.id)) {
      throw new Error(`Duplicate agent id '${definition.id}'`);
    }
    seen.add(definition.id);
  }
  return {
    enabled,
    combined_pane: booleanOrDefault(raw.combined_pane, true),
    disable_in_multi_user_mode: booleanOrDefault(raw.disable_in_multi_user_mode, true),
    definitions,
  };
}

function normalizeHostSwitcher(raw: HostSwitcherConfig | undefined): Required<HostSwitcherConfig> {
  return {
    enabled: raw?.enabled === true,
    suffixes: Array.isArray(raw?.suffixes) ? raw!.suffixes.filter(suffix => typeof suffix === 'string' && suffix.trim()) : [],
    hosts: Array.isArray(raw?.hosts)
      ? raw!.hosts.filter(host => typeof host?.id === 'string' && typeof host?.hostname === 'string')
      : [],
  };
}

export function normalizeAppConfig(config: AppConfig): AppConfig {
  const agents = normalizeAgentsConfig(config);
  const ui = config.app.ui ?? {};
  return {
    app: {
      ...config.app,
      terminal_grid: {
        ...config.app.terminal_grid,
        max_cols: config.app.terminal_grid?.max_cols ?? null,
        max_rows: config.app.terminal_grid?.max_rows ?? null,
      },
      ui: {
        default_pane: stringOrDefault(ui.default_pane, 'terminals'),
        host_switcher: normalizeHostSwitcher(ui.host_switcher),
      },
      agents,
    },
  };
}
