import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import * as yaml from 'js-yaml';
import chokidar, { FSWatcher } from 'chokidar';
import {
  AppConfig, AuthConfig, HostsConfig, LayoutConfig, KeysConfig, Session
} from '../types';

const WEBMUX_ROOT = process.env.WEBMUX_ROOT || path.join(__dirname, '../../..');
const WEBMUX_HOME = process.env.WEBMUX_HOME || path.join(os.homedir(), '.config', 'webmux');
const DEFAULTS_DIR = path.join(WEBMUX_ROOT, 'config.defaults');
const CONFIG_DIR = path.join(WEBMUX_HOME, 'config');
const DATA_DIR = path.join(WEBMUX_HOME, 'data');
const SESSIONS_DIR = path.join(DATA_DIR, 'sessions');
const EVENTS_DIR = path.join(DATA_DIR, 'events');

export const LOGS_DIR = path.join(WEBMUX_HOME, 'logs');

function ensureDirs(): void {
  [CONFIG_DIR, DATA_DIR, SESSIONS_DIR, EVENTS_DIR, LOGS_DIR].forEach(d => {
    if (!fs.existsSync(d)) fs.mkdirSync(d, { recursive: true });
  });

  // Ensure TLS directory exists
  const tlsDir = path.join(CONFIG_DIR, 'tls');
  if (!fs.existsSync(tlsDir)) fs.mkdirSync(tlsDir, { recursive: true });

  // Copy default config files if they don't exist yet
  if (fs.existsSync(DEFAULTS_DIR)) {
    for (const entry of fs.readdirSync(DEFAULTS_DIR)) {
      const src = path.join(DEFAULTS_DIR, entry);
      const dest = path.join(CONFIG_DIR, entry);
      if (fs.statSync(src).isFile() && !fs.existsSync(dest)) {
        fs.copyFileSync(src, dest);
      }
    }
  }
}

function readYaml<T>(file: string): T {
  const content = fs.readFileSync(file, 'utf-8');
  return yaml.load(content) as T;
}

function writeYaml(file: string, data: unknown): void {
  const tmp = file + '.tmp';
  fs.writeFileSync(tmp, yaml.dump(data, { lineWidth: 120 }), 'utf-8');
  fs.renameSync(tmp, file);
}

export class PersistenceManager {
  private watchers: FSWatcher[] = [];
  private changeHandlers: Map<string, (() => void)[]> = new Map();

  constructor() {
    ensureDirs();
  }

  configPath(name: string): string {
    return path.join(CONFIG_DIR, name);
  }

  loadApp(): AppConfig {
    return readYaml<AppConfig>(this.configPath('app.yaml'));
  }

  saveApp(config: AppConfig): void {
    writeYaml(this.configPath('app.yaml'), config);
  }

  loadAuth(): AuthConfig {
    return readYaml<AuthConfig>(this.configPath('auth.yaml'));
  }

  saveAuth(config: AuthConfig): void {
    writeYaml(this.configPath('auth.yaml'), config);
  }

  loadHosts(): HostsConfig {
    return readYaml<HostsConfig>(this.configPath('hosts.yaml'));
  }

  saveHosts(config: HostsConfig): void {
    writeYaml(this.configPath('hosts.yaml'), config);
  }

  loadLayout(): LayoutConfig {
    return readYaml<LayoutConfig>(this.configPath('layout.yaml'));
  }

  saveLayout(config: LayoutConfig): void {
    writeYaml(this.configPath('layout.yaml'), config);
  }

  loadKeys(): KeysConfig {
    return readYaml<KeysConfig>(this.configPath('keys.yaml'));
  }

  saveKeys(config: KeysConfig): void {
    writeYaml(this.configPath('keys.yaml'), config);
  }

  saveSessions(sessions: Session[]): void {
    writeYaml(path.join(SESSIONS_DIR, 'sessions.yaml'), { sessions });
  }

  loadSessions(): Session[] {
    const file = path.join(SESSIONS_DIR, 'sessions.yaml');
    if (!fs.existsSync(file)) return [];
    try {
      const data = readYaml<{ sessions: Session[] }>(file);
      return data.sessions || [];
    } catch {
      return [];
    }
  }

  appendEvent(event: Record<string, unknown>): void {
    const today = new Date().toISOString().slice(0, 10);
    const file = path.join(EVENTS_DIR, `events-${today}.jsonl`);
    const line = JSON.stringify({ ...event, ts: new Date().toISOString() }) + '\n';
    fs.appendFileSync(file, line, 'utf-8');
  }

  watchConfig(filename: string, handler: () => void): void {
    const file = this.configPath(filename);
    if (!this.changeHandlers.has(file)) {
      this.changeHandlers.set(file, []);
    }
    this.changeHandlers.get(file)!.push(handler);

    const watcher = chokidar.watch(file, { persistent: false, ignoreInitial: true });
    watcher.on('change', () => {
      const handlers = this.changeHandlers.get(file) || [];
      handlers.forEach(h => {
        try { h(); } catch (e) { console.error('Config change handler error:', e); }
      });
    });
    this.watchers.push(watcher);
  }

  close(): void {
    this.watchers.forEach(w => w.close());
  }
}

export const persistence = new PersistenceManager();
