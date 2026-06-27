import * as path from 'path';
import type {
  AgentDefinition,
  AgentDefinitionConfig,
  AppConfig,
  AppFontFaceConfig,
  HostSwitcherConfig,
  NormalizedAgentsConfig,
  WorkspaceName,
} from '../types';

const AGENT_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/;
const TMUX_SOCKET_RE = /^[a-zA-Z0-9_.-]+$/;
export const DEFAULT_TERMINAL_FONT_FAMILY = 'ui-monospace, "SFMono-Regular", Monaco, Menlo, Consolas, "Liberation Mono", "DejaVu Sans Mono", monospace';
export const FONT_FACE_CONFIG_ERROR = 'Invalid app.font_faces';
const SUPPORTED_FONT_EXTENSIONS = new Set(['.otf', '.ttf', '.woff', '.woff2']);
const FONT_DISPLAY_VALUES = new Set(['auto', 'block', 'swap', 'fallback', 'optional']);
const GENERIC_FONT_FAMILIES = new Set([
  'serif',
  'sans-serif',
  'monospace',
  'cursive',
  'fantasy',
  'system-ui',
  'ui-serif',
  'ui-sans-serif',
  'ui-monospace',
  'ui-rounded',
  'emoji',
  'math',
  'fangsong',
]);

function stringOrDefault(value: unknown, fallback: string): string {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function booleanOrDefault(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

function splitFontFamilyList(fontFamily: string): string[] {
  const parts: string[] = [];
  let current = '';
  let quote: '"' | "'" | null = null;

  for (let i = 0; i < fontFamily.length; i += 1) {
    const char = fontFamily[i];
    if (quote) {
      current += char;
      if (char === '\\' && i + 1 < fontFamily.length) {
        i += 1;
        current += fontFamily[i];
      } else if (char === quote) {
        quote = null;
      }
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
      current += char;
      continue;
    }
    if (char === ',') {
      parts.push(current.trim());
      current = '';
      continue;
    }
    current += char;
  }
  if (quote) throw new Error('Invalid app.default_term.font_family');
  parts.push(current.trim());
  if (parts.some(part => !part)) throw new Error('Invalid app.default_term.font_family');
  return parts;
}

function normalizeFontFamilyPart(part: string): string {
  const quote = part[0];
  if ((quote === '"' || quote === "'") && part[part.length - 1] === quote) return part;
  if (part.includes('"') || part.includes("'") || part.includes('\\')) {
    throw new Error('Invalid app.default_term.font_family');
  }
  if (GENERIC_FONT_FAMILIES.has(part.toLowerCase()) || !/\s/.test(part)) return part;
  return `"${part}"`;
}

function normalizeFontFamily(value: unknown): string {
  const fontFamily = stringOrDefault(value, DEFAULT_TERMINAL_FONT_FAMILY);
  if (fontFamily.length > 256 || /[\0\r\n;{}]/.test(fontFamily)) {
    throw new Error('Invalid app.default_term.font_family');
  }
  const normalized = splitFontFamilyList(fontFamily).map(normalizeFontFamilyPart).join(', ');
  if (normalized.length > 256) {
    throw new Error('Invalid app.default_term.font_family');
  }
  return normalized;
}

function stripMatchingQuotes(value: string): string {
  const quote = value[0];
  if ((quote === '"' || quote === "'") && value[value.length - 1] === quote) {
    return value.slice(1, -1);
  }
  return value;
}

function normalizeFontFaceFamily(value: unknown): string {
  const family = stringOrDefault(value, '');
  if (!family || family.length > 128 || /[\0\r\n;{}\\]/.test(family)) {
    throw new Error(FONT_FACE_CONFIG_ERROR);
  }
  return stripMatchingQuotes(normalizeFontFamilyPart(family));
}

function normalizeFontFaceSource(value: unknown): string {
  const source = stringOrDefault(value, '');
  if (
    !source ||
    source.length > 512 ||
    path.isAbsolute(source) ||
    /[\0\r\n]/.test(source) ||
    /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(source)
  ) {
    throw new Error(FONT_FACE_CONFIG_ERROR);
  }
  const extension = path.extname(source).toLowerCase();
  if (!SUPPORTED_FONT_EXTENSIONS.has(extension)) {
    throw new Error(FONT_FACE_CONFIG_ERROR);
  }
  return source;
}

function normalizeFontFaceWeight(value: unknown): string | undefined {
  if (value === undefined || value === null || value === '') return undefined;
  const weight = typeof value === 'number' ? String(value) : stringOrDefault(value, '');
  if (/^(normal|bold|[1-9]00)$/.test(weight)) return weight;
  throw new Error(FONT_FACE_CONFIG_ERROR);
}

function normalizeFontFaceStyle(value: unknown): string | undefined {
  if (value === undefined || value === null || value === '') return undefined;
  const style = stringOrDefault(value, '').toLowerCase();
  if (style === 'normal' || style === 'italic' || style === 'oblique') return style;
  throw new Error(FONT_FACE_CONFIG_ERROR);
}

function normalizeFontFaceDisplay(value: unknown): AppFontFaceConfig['display'] | undefined {
  if (value === undefined || value === null || value === '') return undefined;
  const display = stringOrDefault(value, '').toLowerCase();
  if (FONT_DISPLAY_VALUES.has(display)) return display as AppFontFaceConfig['display'];
  throw new Error(FONT_FACE_CONFIG_ERROR);
}

export function normalizeFontFaces(value: unknown): AppFontFaceConfig[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.length > 32) {
    throw new Error(FONT_FACE_CONFIG_ERROR);
  }
  return value.map(face => {
    if (!face || typeof face !== 'object') {
      throw new Error(FONT_FACE_CONFIG_ERROR);
    }
    const raw = face as Record<string, unknown>;
    const normalized: AppFontFaceConfig = {
      family: normalizeFontFaceFamily(raw.family),
      source: normalizeFontFaceSource(raw.source),
    };
    const weight = normalizeFontFaceWeight(raw.weight);
    const style = normalizeFontFaceStyle(raw.style);
    const display = normalizeFontFaceDisplay(raw.display);
    if (weight) normalized.weight = weight;
    if (style) normalized.style = style;
    if (display) normalized.display = display;
    return normalized;
  });
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
      default_term: {
        ...config.app.default_term,
        font_family: normalizeFontFamily(config.app.default_term?.font_family),
      },
      font_faces: normalizeFontFaces(config.app.font_faces),
      ui: {
        default_pane: stringOrDefault(ui.default_pane, 'terminals'),
        host_switcher: normalizeHostSwitcher(ui.host_switcher),
      },
      agents,
    },
  };
}
