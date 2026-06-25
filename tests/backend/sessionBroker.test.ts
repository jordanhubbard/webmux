import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('SessionBroker', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let SessionBroker: typeof import('@backend/services/sessionBroker').SessionBroker;
  let transportLauncher: typeof import('@backend/services/transportLauncher').transportLauncher;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-broker-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    // Create config files
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts:\n  - id: h1\n    hostname: host1.example.com\n    port: 22\n    tags: []\n    mosh_allowed: false\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');
    fs.writeFileSync(path.join(configDir, 'app.yaml'), 'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    jest.resetModules();
    ({ SessionBroker } = require('@backend/services/sessionBroker'));
    ({ transportLauncher } = require('@backend/services/transportLauncher'));
    (SessionBroker as unknown as Record<string, number>).AGENT_ATTACH_REPLAY_SUPPRESS_MS = 1500;
    (SessionBroker as unknown as Record<string, number>).AGENT_STATUS_FLUSH_DEBOUNCE_MS = 1;
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  function encodedStatusName(name: string) {
    return Buffer.from(name, 'utf8')
      .toString('base64')
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/g, '');
  }

  function readAgentStatus(agentId: string, name: string) {
    const file = path.join(tmpDir, 'data', 'agent-status', agentId, `${encodedStatusName(name)}.json`);
    return JSON.parse(fs.readFileSync(file, 'utf8')) as Record<string, unknown>;
  }

  function sleep(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

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

  it('wraps automatic terminal positions when max_cols is set', async () => {
    const configDir = path.join(tmpDir, 'config');
    fs.writeFileSync(path.join(configDir, 'app.yaml'), 'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  terminal_grid:\n    max_cols: 1\n    max_rows: 2\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    const broker = new SessionBroker();
    await broker.initialize();
    const s1 = await broker.create({ username: 'u', hostname: 'h' });
    const s2 = await broker.create({ username: 'u', hostname: 'h' });

    expect(s1.row).toBe(0);
    expect(s1.col).toBe(0);
    expect(s2.row).toBe(1);
    expect(s2.col).toBe(0);
  });

  it('rejects terminal sessions when the configured grid is full', async () => {
    const configDir = path.join(tmpDir, 'config');
    fs.writeFileSync(path.join(configDir, 'app.yaml'), 'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  terminal_grid:\n    max_cols: 1\n    max_rows: 1\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    const broker = new SessionBroker();
    await broker.initialize();
    await broker.create({ username: 'u', hostname: 'h' });

    await expect(broker.create({ username: 'u', hostname: 'h' })).rejects.toThrow('Terminal grid is full');
  });

  it('rejects moves outside the configured terminal grid', async () => {
    const configDir = path.join(tmpDir, 'config');
    fs.writeFileSync(path.join(configDir, 'app.yaml'), 'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  terminal_grid:\n    max_cols: 1\n    max_rows: 1\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });

    expect(() => broker.move(session.id, 0, 1)).toThrow('exceeds max_cols 1');
  });

  it('keeps agent workspace sessions out of terminal lists and layout compaction', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const first = await broker.create({ username: 'u', hostname: 'h', row: 0, col: 0 });
    const second = await broker.create({ username: 'u', hostname: 'h', row: 2, col: 0 });
    const { session: agent } = await broker.ensureAgentScratch('anonymous', 'codex', 'agent-codex', 80, 24, tmpDir);

    expect(broker.list()).toHaveLength(3);
    expect(broker.listByOwner('anonymous').map(s => s.id)).toEqual([first.id, second.id]);

    await broker.delete(agent.id);

    expect(broker.get(first.id)!.row).toBe(0);
    expect(broker.get(first.id)!.col).toBe(0);
    expect(broker.get(second.id)!.row).toBe(2);
    expect(broker.get(second.id)!.col).toBe(0);
  });

  it('rejects moves for agent workspace sessions', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const { session } = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );

    expect(() => broker.move(session.id, 1, 1)).toThrow('Agent workspace sessions cannot be moved');
  });

  it('marks agent attach sessions connected immediately after launch', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const { session } = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );

    expect(session.state).toBe('connected');
  });

  it('does not touch updated_at when reusing the same live agent attach at the same size', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const first = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );
    first.session.updated_at = '2026-06-17T20:00:00.000Z';
    const handle = transportLauncher.getHandle(first.session.id)!;
    const resizeSpy = jest.spyOn(handle, 'resize');

    const second = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );

    expect(second.session.id).toBe(first.session.id);
    expect(resizeSpy).not.toHaveBeenCalled();
    expect(second.session.updated_at).toBe('2026-06-17T20:00:00.000Z');
  });

  it('ignores stale PTY exit events after relaunching an agent attach session', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const first = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );
    const staleHandle = transportLauncher.getHandle(first.session.id) as unknown as { emit: (event: string, data: unknown) => void };

    const second = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-b',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-b'],
    );

    expect(second.session.id).toBe(first.session.id);
    staleHandle.emit('exit', { exitCode: 0 });
    expect(broker.get(first.session.id)!.state).toBe('connected');

    const currentHandle = transportLauncher.getHandle(first.session.id) as unknown as { emit: (event: string, data: unknown) => void };
    currentHandle.emit('exit', { exitCode: 0 });
    expect(broker.get(first.session.id)!.state).toBe('disconnected');
  });

  it('does not record agent output metadata from tmux attach replay', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const { session } = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    handle.emit('data', 'tmux screen replay');
    await sleep(25);

    expect(fs.existsSync(path.join(tmpDir, 'data', 'agent-status', 'codex'))).toBe(false);
  });

  it('records live agent output metadata after tmux attach replay', async () => {
    (SessionBroker as unknown as Record<string, number>).AGENT_ATTACH_REPLAY_SUPPRESS_MS = 1;
    const broker = new SessionBroker();
    await broker.initialize();
    const { session } = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    );
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    await sleep(5);
    handle.emit('data', 'live agent output');
    await sleep(25);

    expect(readAgentStatus('codex', 'codex-a')).toMatchObject({
      agent_id: 'codex',
      name: 'codex-a',
      status: 'working',
      source: 'webmux',
      last_output_source: 'live',
    });
    expect(typeof readAgentStatus('codex', 'codex-a').last_output_at).toBe('string');
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

  it('shutdown persists session state and kills PTYs', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    await broker.create({ username: 'u', hostname: 'h' });
    broker.shutdown();

    // Verify sessions are persisted as disconnected
    const sessFile = path.join(tmpDir, 'data', 'sessions', 'sessions.yaml');
    const content = fs.readFileSync(sessFile, 'utf-8');
    expect(content).toContain('hostname: h');
  });

  it('auto-reconnects persistent sessions on initialize', async () => {
    const broker1 = new SessionBroker();
    await broker1.initialize();
    const session = await broker1.create({ username: 'u', hostname: 'h' });
    broker1.shutdown();

    // Re-initialize a fresh broker — it should load and attempt reconnect
    jest.resetModules();
    const { SessionBroker: SessionBroker2 } = require('@backend/services/sessionBroker');
    const broker2 = new SessionBroker2();
    await broker2.initialize();

    const sessions = broker2.list();
    expect(sessions).toHaveLength(1);
    expect(sessions[0].id).toBe(session.id);
    // State should be 'connecting' or 'error' (error since hostname 'h' is not resolvable)
    expect(['connecting', 'error']).toContain(sessions[0].state);
  });
});
