import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('SessionBroker', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let SessionBroker: typeof import('@backend/services/sessionBroker').SessionBroker;
  let SessionTranscriptLogger: typeof import('@backend/services/sessionTranscriptLogger').SessionTranscriptLogger;
  let transportLauncher: typeof import('@backend/services/transportLauncher').transportLauncher;
  let persistence: typeof import('@backend/services/persistenceManager').persistence;

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
    ({ SessionTranscriptLogger } = require('@backend/services/sessionTranscriptLogger'));
    ({ transportLauncher } = require('@backend/services/transportLauncher'));
    ({ persistence } = require('@backend/services/persistenceManager'));
    (SessionBroker as unknown as Record<string, number>).AGENT_ATTACH_REPLAY_SUPPRESS_MS = 1500;
    (SessionBroker as unknown as Record<string, number>).AGENT_STATUS_FLUSH_DEBOUNCE_MS = 1;
  });

  afterEach(async () => {
    await persistence.close();
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  });

  function encodedStatusName(name: string) {
    return Buffer.from(name, 'utf8')
      .toString('base64')
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/g, '');
  }

  async function readAgentStatus(agentId: string, name: string) {
    const file = path.join(tmpDir, 'data', 'agent-status', agentId, `${encodedStatusName(name)}.json`);
    const deadline = Date.now() + 3000;
    for (;;) {
      try {
        return JSON.parse(await fs.promises.readFile(file, 'utf8')) as Record<string, unknown>;
      } catch (error) {
        // Status publication is asynchronous and atomic. Wait for publication,
        // not an assumed filesystem latency on the hosted Windows runner.
        if ((error as NodeJS.ErrnoException).code !== 'ENOENT' || Date.now() >= deadline) throw error;
        await sleep(10);
      }
    }
  }

  function sleep(ms: number) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  function enableTranscriptLogging(): void {
    fs.appendFileSync(
      path.join(tmpDir, 'config', 'app.yaml'),
      '  session_logging:\n    enabled: true\n',
    );
  }

  function transcriptFiles(): string[] {
    const directory = path.join(tmpDir, 'logs', 'sessions');
    return fs.existsSync(directory) ? fs.readdirSync(directory).sort() : [];
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

  it('relaunches a live agent attach when the exec argv changes', async () => {
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
    const staleHandle = transportLauncher.getHandle(first.session.id) as unknown as {
      emit: (event: string, data: unknown) => void;
      kill: () => void;
    };
    const killSpy = jest.spyOn(staleHandle, 'kill');

    const second = await broker.ensureAgentAttach(
      'anonymous',
      'codex',
      'agent-codex',
      'codex-a',
      80,
      24,
      ['tmux', '-L', 'codex-alt', 'attach-session', '-t', 'codex-a'],
    );

    expect(second.session.id).toBe(first.session.id);
    expect(second.session.exec_argv).toEqual(['tmux', '-L', 'codex-alt', 'attach-session', '-t', 'codex-a']);
    expect(killSpy).toHaveBeenCalledTimes(1);
    expect(transportLauncher.getHandle(first.session.id)).not.toBe(staleHandle);

    staleHandle.emit('exit', { exitCode: 0 });
    expect(broker.get(first.session.id)!.state).toBe('connected');
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

    const status = await readAgentStatus('codex', 'codex-a');
    expect(status).toMatchObject({
      agent_id: 'codex',
      name: 'codex-a',
      status: 'working',
      source: 'webmux',
      last_output_source: 'live',
    });
    expect(typeof status.last_output_at).toBe('string');
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

  it('does not create transcripts when session logging is disabled', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    handle.emit('data', 'not persisted');
    await broker.shutdown();

    expect(transcriptFiles()).toEqual([]);
  });

  it('logs PTY output and transcript lifecycle events when enabled', async () => {
    enableTranscriptLogging();
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    handle.emit('data', 'hello from the PTY\r\n');
    const files = transcriptFiles();
    expect(files).toHaveLength(1);
    expect(files[0]).toMatch(new RegExp(`^session-${session.id}-\\d{8}T\\d{6}Z-g1-[a-f0-9]{8}\\.log$`));
    const file = path.join(tmpDir, 'logs', 'sessions', files[0]);
    await broker.flushTranscript(session.id);
    const liveContent = fs.readFileSync(file, 'utf8');
    expect(liveContent).toContain('[webmux transcript started');
    expect(liveContent).toContain('hello from the PTY');

    await broker.shutdown();

    expect(fs.readFileSync(file, 'utf8')).toContain('reason=shutdown');
    if (process.platform !== 'win32') {
      expect(fs.statSync(file).mode & 0o777).toBe(0o600);
      expect(fs.statSync(path.dirname(file)).mode & 0o777).toBe(0o700);
    }

    const eventsDir = path.join(tmpDir, 'data', 'events');
    const eventLines = fs.readFileSync(path.join(eventsDir, fs.readdirSync(eventsDir)[0]), 'utf8')
      .trim()
      .split('\n')
      .map(line => JSON.parse(line) as Record<string, unknown>);
    expect(eventLines).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'session_transcript_started', session_id: session.id, path: file }),
      expect.objectContaining({ type: 'session_transcript_stopped', session_id: session.id, path: file, reason: 'shutdown' }),
    ]));
  });

  it('pauses and resumes the same launch transcript, recreating it if removed', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    const started = await broker.toggleTranscript(session.id);
    expect(started.enabled).toBe(true);
    expect(started.file).toBeDefined();
    handle.emit('data', 'before pause\r\n');
    await broker.flushTranscript(session.id);

    const paused = await broker.toggleTranscript(session.id);
    expect(paused).toEqual({ enabled: false, file: started.file });
    handle.emit('data', 'while paused\r\n');
    expect(fs.readFileSync(started.file!, 'utf8')).not.toContain('while paused');

    const resumed = await broker.toggleTranscript(session.id);
    expect(resumed).toEqual({ enabled: true, file: started.file });
    handle.emit('data', 'after resume\r\n');
    await broker.flushTranscript(session.id);
    const resumedContent = fs.readFileSync(started.file!, 'utf8');
    expect(resumedContent).toContain('before pause');
    expect(resumedContent).not.toContain('while paused');
    expect(resumedContent).toContain('[webmux transcript resumed');
    expect(resumedContent).toContain('after resume');
    expect(transcriptFiles()).toHaveLength(1);

    await broker.toggleTranscript(session.id);
    fs.unlinkSync(started.file!);
    const recreated = await broker.toggleTranscript(session.id);
    expect(recreated).toEqual({ enabled: true, file: started.file });
    handle.emit('data', 'after recreation\r\n');
    await broker.flushTranscript(session.id);
    expect(fs.readFileSync(started.file!, 'utf8')).toContain('after recreation');
    await broker.shutdown();
  });

  it('uses a new transcript file after reconnecting', async () => {
    enableTranscriptLogging();
    const broker = new SessionBroker();
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const firstHandle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };
    firstHandle.emit('data', 'first launch output\r\n');

    await broker.reconnect(session.id);
    const secondHandle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };
    secondHandle.emit('data', 'second launch output\r\n');
    await broker.shutdown();

    const contents = transcriptFiles().map(file =>
      fs.readFileSync(path.join(tmpDir, 'logs', 'sessions', file), 'utf8'),
    );
    expect(contents).toHaveLength(2);
    expect(contents.filter(content => content.includes('first launch output'))).toHaveLength(1);
    expect(contents.filter(content => content.includes('second launch output'))).toHaveLength(1);
  });

  it('keeps the session running when a transcript write fails', async () => {
    enableTranscriptLogging();
    let writes = 0;
    const close = jest.fn(async () => undefined);
    const logger = new SessionTranscriptLogger(() => ({
      write: () => {
        writes += 1;
        if (writes > 1) throw new Error('disk full');
      },
      close,
    }));
    const error = jest.spyOn(console, 'error').mockImplementation(() => undefined);
    const broker = new SessionBroker(logger);
    await broker.initialize();
    const session = await broker.create({ username: 'u', hostname: 'h' });
    const handle = transportLauncher.getHandle(session.id) as unknown as { emit: (event: string, data: unknown) => void };

    handle.emit('data', 'still reaches the session');

    expect(broker.get(session.id)?.state).toBe('connected');
    expect(error).toHaveBeenCalledWith(
      `Transcript logging failed for session ${session.id}:`,
      expect.objectContaining({ message: 'disk full' }),
    );
    await broker.shutdown();
    expect(close).toHaveBeenCalled();
    error.mockRestore();
  });

  it('shutdown persists session state and kills PTYs', async () => {
    const broker = new SessionBroker();
    await broker.initialize();
    await broker.create({ username: 'u', hostname: 'h' });
    await broker.shutdown();

    // Verify sessions are persisted as disconnected
    const sessFile = path.join(tmpDir, 'data', 'sessions', 'sessions.yaml');
    const content = fs.readFileSync(sessFile, 'utf-8');
    expect(content).toContain('hostname: h');
  });

  it('auto-reconnects persistent sessions on initialize', async () => {
    const broker1 = new SessionBroker();
    await broker1.initialize();
    const session = await broker1.create({ username: 'u', hostname: 'h' });
    await broker1.shutdown();

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
