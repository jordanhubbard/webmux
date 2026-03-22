import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

const APP_YAML = `app:
  name: webmux
  listen_host: 0.0.0.0
  http_port: 8080
  https_port: 8443
  secure_mode: false
  trusted_http_allowed: true
  default_term:
    cols: 80
    rows: 24
    font_size: 14
  transport:
    prefer_mosh: false
    ssh_fallback: true
    mosh_server_path: ""
ai:
  enabled: true
  rcc_url: http://localhost:9999
  rcc_token: ""
  context_lines: 50
`;

describe('Claude session type', () => {
  let tmpDir: string;
  let SessionBroker: typeof import('@backend/services/sessionBroker').SessionBroker;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-claude-'));
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'app.yaml'), APP_YAML);
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts: []\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');

    jest.resetModules();
    ({ SessionBroker } = require('@backend/services/sessionBroker'));
  });

  afterEach(() => {
    process.env.WEBMUX_HOME = undefined;
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('creates a claude session with session_type set', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ session_type: 'claude' }, 'alice');
    expect(session.session_type).toBe('claude');
    expect(session.hostname).toBe('localhost');
    expect(session.title).toBe('Claude CLI');
    expect(broker.list()).toHaveLength(1);
  });

  it('claude session defaults username to "claude"', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ session_type: 'claude' });
    expect(session.username).toBe('claude');
  });

  it('claude session is not persistent', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ session_type: 'claude' });
    expect(session.persistent).toBe(false);
  });

  it('normal SSH session still works with username', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'user1', hostname: 'box.example.com' });
    expect(session.session_type).toBeUndefined();
    expect(session.hostname).toBe('box.example.com');
    expect(session.username).toBe('user1');
  });

  it('claude session reconnect uses launchLocal', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ session_type: 'claude' });
    // Reconnect should not throw for claude sessions
    await expect(broker.reconnect(session.id)).resolves.toBeDefined();
  });
});

describe('TransportLauncher.launchLocal', () => {
  let TransportLauncher: typeof import('@backend/services/transportLauncher').TransportLauncher;

  beforeEach(() => {
    jest.resetModules();
    ({ TransportLauncher } = require('@backend/services/transportLauncher'));
  });

  it('stores a pty handle for the session id', () => {
    const launcher = new TransportLauncher();
    const handle = launcher.launchLocal('test-id', 'claude', [], 80, 24);
    expect(handle).toBeDefined();
    expect(launcher.isAlive('test-id')).toBe(true);
    launcher.kill('test-id');
    expect(launcher.isAlive('test-id')).toBe(false);
  });

  it('throws for invalid command names', () => {
    const launcher = new TransportLauncher();
    expect(() => launcher.launchLocal('id', '../evil', [], 80, 24)).toThrow('Invalid command name');
    expect(() => launcher.launchLocal('id', 'rm -rf /', [], 80, 24)).toThrow('Invalid command name');
  });
});

describe('AI config in AppConfig', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-aiconfig-'));
    process.env.WEBMUX_HOME = tmpDir;
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'app.yaml'), APP_YAML);
    jest.resetModules();
  });

  afterEach(() => {
    process.env.WEBMUX_HOME = undefined;
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('loads AI config from app.yaml', () => {
    const { persistence } = require('@backend/services/persistenceManager');
    const config = persistence.loadApp();
    expect(config.ai).toBeDefined();
    expect(config.ai?.enabled).toBe(true);
    expect(config.ai?.rcc_url).toBe('http://localhost:9999');
    expect(config.ai?.context_lines).toBe(50);
  });
});
