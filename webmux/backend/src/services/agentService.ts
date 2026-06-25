import { execFile } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import type {
  AgentDefinition,
  AgentRuntimeStatus,
  AgentStatusSource,
  AgentTmuxSession,
  NormalizedAgentsConfig,
} from '../types';
import { DATA_DIR, persistence } from './persistenceManager';
import { normalizeAgentsConfig } from './appConfig';

const STATUS_STALE_MS = 24 * 60 * 60 * 1000;
const STATUS_RECENT_MS = 5 * 60 * 1000;
const STATUS_ACTIVITY_SLOP_MS = 1500;

interface AgentStatusMetadata {
  agent_id?: string;
  name?: string;
  status?: AgentRuntimeStatus;
  source?: AgentStatusSource;
  updated_at?: string;
  last_input_at?: string;
  last_output_at?: string;
  last_output_source?: 'live';
  last_ready_at?: string;
}

function isNoTmuxServerError(err: unknown): boolean {
  const error = err as NodeJS.ErrnoException & { stderr?: string };
  const text = `${error.message ?? ''}\n${error.stderr ?? ''}`.toLowerCase();
  return text.includes('no server running') ||
    (text.includes('error connecting to') && text.includes('no such file or directory'));
}

function epochSecondsToIso(raw: string | undefined): string | undefined {
  const value = Number(raw);
  if (!Number.isFinite(value) || value <= 0) return undefined;
  return new Date(value * 1000).toISOString();
}

function isoTime(value: string | undefined): number {
  if (!value) return 0;
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : 0;
}

function latestIso(...values: (string | undefined)[]): string | undefined {
  let latest = 0;
  let latestValue: string | undefined;
  for (const value of values) {
    const time = isoTime(value);
    if (time > latest) {
      latest = time;
      latestValue = value;
    }
  }
  return latestValue;
}

function metadataLastOutput(metadata: AgentStatusMetadata | undefined): string | undefined {
  if (metadata?.source === 'webmux' && metadata.last_output_source !== 'live') return undefined;
  return metadata?.last_output_at;
}

function statusFileName(name: string): string {
  return Buffer.from(name, 'utf8')
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '');
}

function statusDir(agentId: string): string {
  return path.join(DATA_DIR, 'agent-status', agentId);
}

function statusPath(agentId: string, name: string): string {
  return path.join(statusDir(agentId), `${statusFileName(name)}.json`);
}

function normalizeDisplayBase(agentId: string, name: string): string {
  const withoutPrefix = name.startsWith(`${agentId}-`) ? name.slice(agentId.length + 1) : name;
  return withoutPrefix.replace(/-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}$/, '') || name;
}

export class AgentService {
  getRuntimeConfig(): NormalizedAgentsConfig {
    return normalizeAgentsConfig(persistence.loadApp());
  }

  getDefinitions(): AgentDefinition[] {
    const config = this.getRuntimeConfig();
    return config.enabled ? config.definitions : [];
  }

  getConfig(agentId: string): AgentDefinition | undefined {
    return this.getDefinitions().find(definition => definition.id === agentId);
  }

  parseTmuxSessions(output: string, definition: AgentDefinition): AgentTmuxSession[] {
    const sessions = output
      .split(/\r?\n/)
      .map(line => line.trim())
      .filter(Boolean)
      .map(line => {
        const [name, windowsRaw, attachedRaw, createdRaw, activityRaw] = line.split('\t');
        const createdAt = epochSecondsToIso(createdRaw);
        const activityAt = epochSecondsToIso(activityRaw);
        return {
          name,
          agent_id: definition.id,
          display_name: normalizeDisplayBase(definition.id, name),
          windows: Number(windowsRaw) || 0,
          attached: Number(attachedRaw) || 0,
          created_at: createdAt,
          last_output_at: activityAt,
          status: this.inferStatus(activityAt),
          status_source: activityAt ? 'tmux' as AgentStatusSource : 'none' as AgentStatusSource,
        };
      })
      .filter(session => session.name.length > 0);
    return this.assignDisplayNames(sessions);
  }

  private execFileOutput(command: string, args: string[]): Promise<string> {
    return new Promise((resolve, reject) => {
      execFile(command, args, { encoding: 'utf8' }, (err, stdout, stderr) => {
        if (err) {
          (err as NodeJS.ErrnoException & { stderr?: string }).stderr = String(stderr ?? '');
          reject(err);
          return;
        }
        resolve(String(stdout));
      });
    });
  }

  private tmuxSocketArgs(definition: AgentDefinition): string[] {
    return path.isAbsolute(definition.tmux_socket)
      ? ['-S', definition.tmux_socket]
      : ['-L', definition.tmux_socket];
  }

  private assignDisplayNames(sessions: AgentTmuxSession[]): AgentTmuxSession[] {
    const byBase = new Map<string, AgentTmuxSession[]>();
    for (const session of sessions) {
      const base = normalizeDisplayBase(session.agent_id, session.name);
      if (!byBase.has(base)) byBase.set(base, []);
      byBase.get(base)!.push(session);
      session.display_name = base;
    }

    for (const [base, group] of byBase) {
      if (group.length <= 1) continue;
      const ordered = [...group].sort((a, b) => {
        const createdDiff = isoTime(a.created_at) - isoTime(b.created_at);
        return createdDiff || a.agent_id.localeCompare(b.agent_id) || a.name.localeCompare(b.name);
      });
      ordered.forEach((session, index) => {
        session.display_name = `${base} (${index + 1})`;
      });
    }

    return sessions;
  }

  private inferStatus(lastOutputAt: string | undefined, metadata?: AgentStatusMetadata): AgentRuntimeStatus {
    const now = Date.now();
    const lastOutputMs = isoTime(lastOutputAt);
    const metadataUpdatedMs = isoTime(metadata?.updated_at);

    if (metadata?.status === 'waiting') {
      if (!lastOutputMs || metadataUpdatedMs + STATUS_ACTIVITY_SLOP_MS >= lastOutputMs) {
        return 'waiting';
      }
      return 'working';
    }

    if (metadata?.status === 'working') return 'working';

    if (lastOutputMs && now - lastOutputMs <= STATUS_RECENT_MS) return 'working';
    if (!metadata?.status && lastOutputMs && now - lastOutputMs >= STATUS_STALE_MS) return 'stale';
    return 'unknown';
  }

  private inferStatusSource(lastOutputAt: string | undefined, metadata?: AgentStatusMetadata): AgentStatusSource {
    const metadataUpdatedMs = isoTime(metadata?.updated_at);
    const lastOutputMs = isoTime(lastOutputAt);
    if (metadata?.status === 'waiting' && (!lastOutputMs || metadataUpdatedMs + STATUS_ACTIVITY_SLOP_MS >= lastOutputMs)) {
      return metadata.source ?? 'hook';
    }
    if (metadata?.status === 'working') return metadata.source ?? 'webmux';
    return lastOutputAt ? 'tmux' : 'none';
  }

  private async readStatus(agentId: string, name: string): Promise<AgentStatusMetadata | undefined> {
    try {
      const content = await fs.promises.readFile(statusPath(agentId, name), 'utf8');
      const parsed = JSON.parse(content) as AgentStatusMetadata;
      if (parsed.agent_id && parsed.agent_id !== agentId) return undefined;
      if (parsed.name && parsed.name !== name) return undefined;
      return parsed;
    } catch {
      return undefined;
    }
  }

  private async mergeStatusMetadata(session: AgentTmuxSession): Promise<AgentTmuxSession> {
    const metadata = await this.readStatus(session.agent_id, session.name);
    const lastOutputAt = latestIso(session.last_output_at, metadataLastOutput(metadata), metadata?.last_ready_at);
    return {
      ...session,
      last_output_at: lastOutputAt,
      status: this.inferStatus(lastOutputAt, metadata),
      status_source: this.inferStatusSource(lastOutputAt, metadata),
    };
  }

  async recordStatus(
    agentId: string,
    name: string,
    update: {
      status: AgentRuntimeStatus;
      source: AgentStatusSource;
      last_input_at?: string;
      last_output_at?: string;
      last_output_source?: 'live';
      last_ready_at?: string;
    },
  ): Promise<void> {
    const updatedAt = new Date().toISOString();
    const file = statusPath(agentId, name);
    await fs.promises.mkdir(path.dirname(file), { recursive: true });
    const previous = await this.readStatus(agentId, name);
    const lastOutputSource = update.last_output_source ?? (update.last_output_at === undefined ? previous?.last_output_source : undefined);
    const next: AgentStatusMetadata = {
      ...previous,
      agent_id: agentId,
      name,
      status: update.status,
      source: update.source,
      updated_at: updatedAt,
      last_input_at: update.last_input_at ?? previous?.last_input_at,
      last_output_at: update.last_output_at ?? previous?.last_output_at,
      last_output_source: lastOutputSource,
      last_ready_at: update.last_ready_at ?? previous?.last_ready_at,
    };
    const tmp = `${file}.tmp-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    await fs.promises.writeFile(tmp, `${JSON.stringify(next, null, 2)}\n`, 'utf8');
    await fs.promises.rename(tmp, file);
  }

  async listSessions(agentId: string): Promise<AgentTmuxSession[]> {
    const definition = this.getConfig(agentId);
    if (!definition) throw new Error(`Agent '${agentId}' is not configured`);

    try {
      const output = await this.execFileOutput('tmux', [
        ...this.tmuxSocketArgs(definition),
        'list-sessions',
        '-F',
        '#S\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}',
      ]);
      const sessions = this.parseTmuxSessions(output, definition);
      const enriched = await Promise.all(sessions.map(session => this.mergeStatusMetadata(session)));
      return this.assignDisplayNames(enriched);
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code === 'ENOENT') {
        throw new Error('tmux is not installed');
      }
      if (isNoTmuxServerError(err)) return [];
      throw err;
    }
  }

  async listAllSessions(): Promise<AgentTmuxSession[]> {
    const sessions: AgentTmuxSession[] = [];
    for (const definition of this.getDefinitions()) {
      try {
        sessions.push(...await this.listSessions(definition.id));
      } catch (err) {
        console.warn(`Failed to list ${definition.id} sessions for combined agent list:`, err);
      }
    }
    return this.assignDisplayNames(sessions);
  }

  async hasSession(agentId: string, name: string): Promise<boolean> {
    return (await this.listSessions(agentId)).some(session => session.name === name);
  }

  buildAttachExecArgv(agentId: string, name: string): string[] {
    const definition = this.getConfig(agentId);
    if (!definition) throw new Error(`Agent '${agentId}' is not configured`);
    return ['tmux', ...this.tmuxSocketArgs(definition), 'attach-session', '-t', name];
  }

  async getPaneCurrentPath(agentId: string, name: string): Promise<string | undefined> {
    const definition = this.getConfig(agentId);
    if (!definition) return undefined;

    try {
      const output = await this.execFileOutput('tmux', [
        ...this.tmuxSocketArgs(definition),
        'display-message',
        '-p',
        '-t',
        name,
        '#{pane_current_path}',
      ]);
      const cwd = output.trim();
      if (!cwd || !path.isAbsolute(cwd)) return undefined;
      const stat = await fs.promises.stat(cwd);
      return stat.isDirectory() ? cwd : undefined;
    } catch {
      return undefined;
    }
  }
}

export const agentService = new AgentService();
