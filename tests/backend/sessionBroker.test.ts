import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('SessionBroker', () => {
  let tmpDir: string;
  let originalRoot: string | undefined;
  let SessionBroker: typeof import('@backend/services/sessionBroker').SessionBroker;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-broker-'));
    originalRoot = process.env.WEBMUX_ROOT;
    process.env.WEBMUX_ROOT = tmpDir;

    // Create config files
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts:\n  - id: h1\n    hostname: host1.example.com\n    port: 22\n    tags: []\n    mosh_allowed: false\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');
    fs.writeFileSync(path.join(configDir, 'app.yaml'), 'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    jest.resetModules();
    ({ SessionBroker } = require('@backend/services/sessionBroker'));
  });

  afterEach(() => {
    if (originalRoot === undefined) {
      delete process.env.WEBMUX_ROOT;
    } else {
      process.env.WEBMUX_ROOT = originalRoot;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('initializes with no sessions', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    expect(broker.list()).toEqual([]);
  });

  it('creates a session with ad-hoc hostname', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({
      username: 'user1',
      hostname: 'box.example.com',
      port: 2222,
    });
    expect(session.hostname).toBe('box.example.com');
    expect(session.port).toBe(2222);
    expect(session.username).toBe('user1');
    expect(session.transport).toBe('ssh');
    expect(broker.list()).toHaveLength(1);
  });

  it('creates a session with host_id lookup', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({
      username: 'user1',
      host_id: 'h1',
    });
    expect(session.hostname).toBe('host1.example.com');
    expect(session.port).toBe(22);
  });

  it('get returns session by id', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    expect(broker.get(session.id)).toBeDefined();
    expect(broker.get(session.id)!.id).toBe(session.id);
  });

  it('get returns undefined for unknown id', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    expect(broker.get('nonexistent')).toBeUndefined();
  });

  it('deletes a session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    await broker.delete(session.id);
    expect(broker.list()).toHaveLength(0);
    expect(broker.get(session.id)).toBeUndefined();
  });

  it('splitRight returns correct position', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h', row: 0, col: 0 });
    const pos = broker.splitRight(session.id);
    expect(pos).toEqual({ row: 0, col: 1 });
  });

  it('splitBelow returns correct position', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h', row: 0, col: 0 });
    const pos = broker.splitBelow(session.id);
    expect(pos).toEqual({ row: 1, col: 0 });
  });

  it('splitRight throws for unknown session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    expect(() => broker.splitRight('nonexistent')).toThrow();
  });

  it('splitBelow throws for unknown session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    expect(() => broker.splitBelow('nonexistent')).toThrow();
  });

  it('auto-assigns position when not specified', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const s1 = await broker.create({ username: 'u', hostname: 'h' });
    const s2 = await broker.create({ username: 'u', hostname: 'h' });
    // s1 at (0,0), s2 should be at (0,1)
    expect(s1.row).toBe(0);
    expect(s1.col).toBe(0);
    expect(s2.row).toBe(0);
    expect(s2.col).toBe(1);
  });

  it('persists sessions to disk', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    await broker.create({ username: 'u', hostname: 'h' });

    const sessFile = path.join(tmpDir, 'data', 'sessions', 'sessions.yaml');
    expect(fs.existsSync(sessFile)).toBe(true);
    const content = fs.readFileSync(sessFile, 'utf-8');
    expect(content).toContain('hostname: h');
  });

  it('emits session_created event', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const handler = jest.fn();
    broker.on('session_created', handler);
    await broker.create({ username: 'u', hostname: 'h' });
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('emits session_deleted event', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const handler = jest.fn();
    broker.on('session_deleted', handler);
    const session = await broker.create({ username: 'u', hostname: 'h' });
    await broker.delete(session.id);
    expect(handler).toHaveBeenCalledWith(session.id);
  });

  it('reconnect throws for unknown session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    await expect(broker.reconnect('nonexistent')).rejects.toThrow('not found');
  });

  it('reconnect relaunches a disconnected session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const reconnected = await broker.reconnect(session.id);
    expect(reconnected.id).toBe(session.id);
  });

  it('stores key_id on session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h', key_id: 'mykey' });
    expect(session.key_id).toBe('mykey');
  });
});
