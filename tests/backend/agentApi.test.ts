import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import express from 'express';
import request from 'supertest';

const mockExecFile = jest.fn();
const mockExecSync = jest.fn();

jest.mock('child_process', () => ({
  execFile: mockExecFile,
  execSync: mockExecSync,
}));

describe('Agent API Routes', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let originalShell: string | undefined;
  let app: express.Express;
  let sessionBroker: any;
  let transportLauncher: any;
  let dateNowSpy: jest.SpyInstance<number, []>;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-agents-'));
    originalHome = process.env.WEBMUX_HOME;
    originalShell = process.env.SHELL;
    process.env.WEBMUX_HOME = tmpDir;
    process.env.SHELL = '/bin/test-shell';
    dateNowSpy = jest.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-06-17T20:10:00.000Z'));

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'auth.yaml'), 'auth:\n  mode: none\n  users: []\n');
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts: []\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');
    writeAppConfig(true);

    mockExecFile.mockReset();
    mockExecSync.mockReset();
    jest.resetModules();

    const { default: agentsRouter } = require('@backend/api/agents');
    const { default: sessionsRouter } = require('@backend/api/sessions');
    sessionBroker = require('@backend/services/sessionBroker').sessionBroker;
    transportLauncher = require('@backend/services/transportLauncher').transportLauncher;

    await sessionBroker.initialize();

    app = express();
    app.use(express.json());
    app.use('/api/agents', agentsRouter);
    app.use('/api/sessions', sessionsRouter);
  });

  afterEach(() => {
    if (sessionBroker && transportLauncher) {
      for (const session of sessionBroker.list()) {
        transportLauncher.kill(session.id);
      }
    }
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    if (originalShell === undefined) {
      delete process.env.SHELL;
    } else {
      process.env.SHELL = originalShell;
    }
    dateNowSpy.mockRestore();
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  function writeAppConfig(enabled: boolean, disableInMultiUserMode = true) {
    fs.writeFileSync(
      path.join(tmpDir, 'config', 'app.yaml'),
      'app:\n' +
        '  name: webmux\n' +
        '  listen_host: 0.0.0.0\n' +
        '  http_port: 8080\n' +
        '  https_port: 8443\n' +
        '  secure_mode: false\n' +
        '  trusted_http_allowed: true\n' +
        '  default_term:\n' +
        '    cols: 80\n' +
        '    rows: 24\n' +
        '    font_size: 14\n' +
        '  ui:\n' +
        '    default_pane: terminals\n' +
        '    host_switcher:\n' +
        '      enabled: false\n' +
        '      suffixes: []\n' +
        '      hosts: []\n' +
        '  agents:\n' +
        `    enabled: ${enabled}\n` +
        '    combined_pane: true\n' +
        `    disable_in_multi_user_mode: ${disableInMultiUserMode}\n` +
        '    definitions:\n' +
        '      - id: codex\n' +
        '        label: Codex\n' +
        '        plural_label: Codex Sessions\n' +
        '        badge: CODEX\n' +
        '        tmux_socket: codex\n' +
        '  transport:\n' +
        '    prefer_mosh: false\n' +
        '    ssh_fallback: true\n' +
        '    mosh_server_path: ""\n',
    );
  }

  function writeAuthConfig(content: string) {
    fs.writeFileSync(path.join(tmpDir, 'config', 'auth.yaml'), content);
  }

  function authHeader(username = 'a') {
    const { signToken } = require('@backend/middleware/auth');
    return { Authorization: `Bearer ${signToken(username)}` };
  }

  function mockTmux(output: string) {
    mockExecFile.mockImplementation((_cmd: string, args: string[], _options: unknown, callback: (err: Error | null, stdout?: string) => void) => {
      if (args.includes('list-sessions')) {
        callback(null, output);
        return;
      }
      if (args.includes('display-message')) {
        callback(null, `${tmpDir}\n`);
        return;
      }
      callback(null, '');
    });
  }

  it('returns normalized configured agent definitions', async () => {
    const res = await request(app).get('/api/agents/config');

    expect(res.status).toBe(200);
    expect(res.body).toMatchObject({
      enabled: true,
      combined_pane: true,
      disable_in_multi_user_mode: true,
      definitions: [
        {
          id: 'codex',
          label: 'Codex',
          plural_label: 'Codex Sessions',
          badge: 'CODEX',
          tmux_socket: 'codex',
          workspace: 'agent-codex',
          enabled: true,
        },
      ],
    });
  });

  it('lists tmux sessions through the configured socket', async () => {
    mockTmux('codex-alpha-2026-06-14-15-08-55\t1\t2\t1781474936\t1781725677\n');

    const res = await request(app).get('/api/agents/codex/sessions');

    expect(res.status).toBe(200);
    expect(mockExecFile).toHaveBeenCalledWith(
      'tmux',
      ['-L', 'codex', 'list-sessions', '-F', '#S\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}'],
      { encoding: 'utf8' },
      expect.any(Function),
    );
    expect(res.body).toEqual([
      {
        name: 'codex-alpha-2026-06-14-15-08-55',
        agent_id: 'codex',
        display_name: 'alpha',
        windows: 1,
        attached: 2,
        created_at: '2026-06-14T22:08:56.000Z',
        last_output_at: '2026-06-17T19:47:57.000Z',
        status: 'unknown',
        status_source: 'tmux',
      },
    ]);
  });

  it('keeps agent routes disabled when app.agents.enabled is false', async () => {
    writeAppConfig(false);

    const res = await request(app).get('/api/agents/sessions').set(authHeader());

    expect(res.status).toBe(404);
    expect(res.body.error).toBe('Agent sessions are not enabled');
  });

  it('denies agent routes in multi-user mode by default', async () => {
    writeAuthConfig('auth:\n  mode: local\n  users:\n    - username: a\n      password_hash: x\n    - username: b\n      password_hash: y\n');

    const res = await request(app).get('/api/agents/sessions').set(authHeader());

    expect(res.status).toBe(403);
    expect(res.body.error).toBe('Agent sessions are disabled in multi-user mode');
  });

  it('allows explicit multi-user opt-in', async () => {
    writeAppConfig(true, false);
    writeAuthConfig('auth:\n  mode: local\n  users:\n    - username: a\n      password_hash: x\n    - username: b\n      password_hash: y\n');
    mockTmux('');

    const res = await request(app).get('/api/agents/sessions').set(authHeader());

    expect(res.status).toBe(200);
    expect(res.body).toEqual([]);
  });

  it('validates a live tmux session before attaching with exec argv', async () => {
    mockTmux('codex-a\t1\t0\t1781474936\t1781725677\n');

    const res = await request(app)
      .post('/api/agents/codex/attach')
      .send({ name: 'codex-a', cols: 100, rows: 30 });

    expect(res.status).toBe(201);
    expect(res.body).toMatchObject({
      transport: 'exec',
      title: 'codex-a',
      state: 'connected',
      persistent: false,
      workspace: 'agent-codex',
      agent_id: 'codex',
      agent_role: 'attach',
      agent_session_name: 'codex-a',
      exec_argv: ['tmux', '-L', 'codex', 'attach-session', '-t', 'codex-a'],
    });
    expect(res.body.exec_command).toBeUndefined();
  });

  it('rejects attach for a session not present in live tmux output', async () => {
    mockTmux('codex-a\t1\t0\t1781474936\t1781725677\n');

    const res = await request(app)
      .post('/api/agents/codex/attach')
      .send({ name: 'codex-missing' });

    expect(res.status).toBe(404);
    expect(res.body.error).toBe('Codex session not found');
  });

  it('purges existing agent sessions when generic reconnect is denied', async () => {
    mockTmux('codex-a\t1\t0\t1781474936\t1781725677\n');
    const attach = await request(app)
      .post('/api/agents/codex/attach')
      .send({ name: 'codex-a' });
    expect(attach.status).toBe(201);

    writeAppConfig(false);
    const res = await request(app)
      .post(`/api/sessions/${attach.body.id}/reconnect`)
      .send({});

    expect(res.status).toBe(404);
    expect(res.body.error).toBe('Agent sessions are not enabled');
    expect(sessionBroker.get(attach.body.id)).toBeUndefined();
  });

  it('opens scratch shells in the selected tmux pane cwd when available', async () => {
    mockTmux('codex-a\t1\t0\t1781474936\t1781725677\n');

    const res = await request(app)
      .post('/api/agents/codex/scratch')
      .send({ selectedName: 'codex-a', cols: 80, rows: 24 });

    expect(res.status).toBe(201);
    expect(res.body).toMatchObject({
      transport: 'exec',
      title: 'Scratch shell',
      persistent: false,
      workspace: 'agent-codex',
      agent_id: 'codex',
      agent_role: 'scratch',
      exec_argv: ['/bin/test-shell', '-l'],
      exec_cwd: tmpDir,
    });
  });
});
