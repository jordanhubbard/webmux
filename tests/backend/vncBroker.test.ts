import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('VncBroker', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let VncBroker: typeof import('@backend/services/vncBroker').VncBroker;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-vncbroker-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    // Create required config files
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(
      path.join(configDir, 'hosts.yaml'),
      'hosts:\n' +
      '  - id: h1\n' +
      '    hostname: host1.example.com\n' +
      '    port: 22\n' +
      '    tags: []\n' +
      '    mosh_allowed: false\n' +
      '    vnc_enabled: true\n' +
      '    vnc_port: 5901\n'
    );
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');
    fs.writeFileSync(
      path.join(configDir, 'app.yaml'),
      'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n' +
      '  secure_mode: false\n  trusted_http_allowed: true\n' +
      '  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n' +
      '  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n'
    );

    jest.resetModules();
    ({ VncBroker } = require('@backend/services/vncBroker'));
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  // --- initialize ---

  it('initializes with no sessions when no persistence file exists', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    expect(broker.list()).toEqual([]);
  });

  it('initialize() loads persisted sessions and sets them all to disconnected', async () => {
    // Create a broker, add a session, then create a second broker to reload
    const broker1 = new VncBroker();
    await broker1.initialize();
    const session = await broker1.create({ hostname: 'vnc.example.com', vnc_port: 5900 }, 'alice');
    // broker1 saved the session with state 'connecting' — now reload
    jest.resetModules();
    const { VncBroker: VncBroker2 } = require('@backend/services/vncBroker');
    const broker2 = new VncBroker2();
    await broker2.initialize();
    const sessions = broker2.list();
    expect(sessions).toHaveLength(1);
    expect(sessions[0].id).toBe(session.id);
    expect(sessions[0].state).toBe('disconnected');
  });

  // --- create ---

  it('create() assigns a UUID and defaults vnc_port to 5900', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(session.id).toMatch(/^[0-9a-f-]{36}$/);
    expect(session.vnc_port).toBe(5900);
    expect(session.hostname).toBe('vnc.example.com');
  });

  it('create() resolves hostname and vnc_port from host_id when provided', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ host_id: 'h1' }, 'alice');
    expect(session.hostname).toBe('host1.example.com');
    expect(session.vnc_port).toBe(5901);
  });

  it('create() uses provided vnc_port override when no host_id', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com', vnc_port: 5902 }, 'bob');
    expect(session.vnc_port).toBe(5902);
  });

  it('create() assigns grid position (0,0) for first session', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(session.row).toBe(0);
    expect(session.col).toBe(0);
  });

  it('create() assigns next grid position for second session of same owner', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const s1 = await broker.create({ hostname: 'vnc1.example.com' }, 'alice');
    const s2 = await broker.create({ hostname: 'vnc2.example.com' }, 'alice');
    expect(s1.row).toBe(0);
    expect(s1.col).toBe(0);
    expect(s2.row).toBe(0);
    expect(s2.col).toBe(1);
  });

  it('create() scopes grid positions per owner', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const alice1 = await broker.create({ hostname: 'vnc1.example.com' }, 'alice');
    const bob1 = await broker.create({ hostname: 'vnc2.example.com' }, 'bob');
    // Each owner starts their own grid
    expect(alice1.row).toBe(0);
    expect(alice1.col).toBe(0);
    expect(bob1.row).toBe(0);
    expect(bob1.col).toBe(0);
  });

  it('create() stores the password in memory only, not in the persisted session', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create(
      { hostname: 'vnc.example.com', vnc_password: 'secret' },
      'alice'
    );

    // Password is accessible via getPassword
    expect(broker.getPassword(session.id)).toBe('secret');

    // Persisted file must not contain the password
    const sessFile = path.join(tmpDir, 'data', 'sessions', 'vnc-sessions.yaml');
    expect(fs.existsSync(sessFile)).toBe(true);
    const content = fs.readFileSync(sessFile, 'utf-8');
    expect(content).not.toContain('secret');
  });

  it('create() stores kind as vnc', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(session.kind).toBe('vnc');
  });

  it('create() sets state to connecting', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(session.state).toBe('connecting');
  });

  it('create() emits vnc_session_created event', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const handler = jest.fn();
    broker.on('vnc_session_created', handler);
    await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(handler).toHaveBeenCalledTimes(1);
  });

  // --- listByOwner ---

  it('listByOwner() returns only sessions belonging to that owner', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    await broker.create({ hostname: 'vnc1.example.com' }, 'alice');
    await broker.create({ hostname: 'vnc2.example.com' }, 'alice');
    await broker.create({ hostname: 'vnc3.example.com' }, 'bob');

    const aliceSessions = broker.listByOwner('alice');
    const bobSessions = broker.listByOwner('bob');

    expect(aliceSessions).toHaveLength(2);
    expect(aliceSessions.every(s => s.owner === 'alice')).toBe(true);
    expect(bobSessions).toHaveLength(1);
    expect(bobSessions[0].owner).toBe('bob');
  });

  it('listByOwner() returns empty array for unknown owner', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(broker.listByOwner('nobody')).toEqual([]);
  });

  // --- get ---

  it('get() returns session by id', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(broker.get(session.id)).toBeDefined();
    expect(broker.get(session.id)!.id).toBe(session.id);
  });

  it('get() returns undefined for unknown id', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    expect(broker.get('nonexistent')).toBeUndefined();
  });

  // --- delete ---

  it('delete() removes the session', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    await broker.delete(session.id);
    expect(broker.list()).toHaveLength(0);
    expect(broker.get(session.id)).toBeUndefined();
  });

  it('delete() removes the password from memory', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create(
      { hostname: 'vnc.example.com', vnc_password: 'secret' },
      'alice'
    );
    expect(broker.getPassword(session.id)).toBe('secret');
    await broker.delete(session.id);
    expect(broker.getPassword(session.id)).toBeUndefined();
  });

  it('delete() emits vnc_session_deleted event', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const handler = jest.fn();
    broker.on('vnc_session_deleted', handler);
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    await broker.delete(session.id);
    expect(handler).toHaveBeenCalledWith(session.id);
  });

  it('delete() is a no-op for unknown session id', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    // Should not throw
    await expect(broker.delete('nonexistent')).resolves.toBeUndefined();
  });

  // --- move ---

  it('move() updates row and col', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    const updated = broker.move(session.id, 2, 3);
    expect(updated.row).toBe(2);
    expect(updated.col).toBe(3);
    expect(broker.get(session.id)!.row).toBe(2);
    expect(broker.get(session.id)!.col).toBe(3);
  });

  it('move() throws for unknown session', () => {
    const broker = new VncBroker();
    expect(() => broker.move('nonexistent', 0, 0)).toThrow('not found');
  });

  // --- getPassword ---

  it('getPassword() returns undefined when no password was provided', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    expect(broker.getPassword(session.id)).toBeUndefined();
  });

  it('getPassword() returns undefined after delete', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create(
      { hostname: 'vnc.example.com', vnc_password: 'pw' },
      'alice'
    );
    await broker.delete(session.id);
    expect(broker.getPassword(session.id)).toBeUndefined();
  });

  // --- setState ---

  it('setState() updates the session state', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    broker.setState(session.id, 'connected');
    expect(broker.get(session.id)!.state).toBe('connected');
  });

  it('setState() updates to error state', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    const session = await broker.create({ hostname: 'vnc.example.com' }, 'alice');
    broker.setState(session.id, 'error');
    expect(broker.get(session.id)!.state).toBe('error');
  });

  it('setState() is a no-op for unknown session id', async () => {
    const broker = new VncBroker();
    await broker.initialize();
    // Should not throw
    expect(() => broker.setState('nonexistent', 'connected')).not.toThrow();
  });
});
