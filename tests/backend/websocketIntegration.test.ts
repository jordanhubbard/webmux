import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import http from 'http';
import express from 'express';
import WebSocket, { WebSocketServer } from 'ws';
import { EventEmitter } from 'events';

describe('WebSocket Integration', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let server: http.Server;
  let wss: WebSocketServer;
  let port: number;
  let sessionBroker: any;
  let transportLauncher: any;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-ws-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    fs.writeFileSync(path.join(configDir, 'auth.yaml'),
      'auth:\n  mode: none\n  users: []\n');
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts: []\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'), 'layout:\n  font_size: 14\n  tiles: []\n');
    fs.writeFileSync(path.join(configDir, 'app.yaml'),
      'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    jest.resetModules();

    const { default: sessionsRouter } = require('@backend/api/sessions');
    const { default: authRouter } = require('@backend/api/auth');
    const { setupWebSocket } = require('@backend/websocket/handler');
    sessionBroker = require('@backend/services/sessionBroker').sessionBroker;
    transportLauncher = require('@backend/services/transportLauncher').transportLauncher;

    await sessionBroker.initialize();

    const app = express();
    app.use(express.json());
    app.use('/api/sessions', sessionsRouter);
    app.use('/api/auth', authRouter);

    server = http.createServer(app);
    wss = new WebSocketServer({ noServer: true });
    setupWebSocket(wss);

    server.on('upgrade', (request, socket, head) => {
      const pathname = (request.url || '').split('?')[0];
      if (pathname.startsWith('/api/term/')) {
        wss.handleUpgrade(request, socket, head, (ws) => {
          wss.emit('connection', ws, request);
        });
      } else {
        socket.destroy();
      }
    });

    await new Promise<void>((resolve) => {
      server.listen(0, '127.0.0.1', () => {
        const addr = server.address();
        port = typeof addr === 'object' && addr ? addr.port : 0;
        resolve();
      });
    });
  });

  afterEach(async () => {
    for (const s of sessionBroker.list()) {
      transportLauncher.kill(s.id);
    }
    wss.clients.forEach(ws => ws.close());
    wss.close();
    await new Promise<void>((resolve) => server.close(() => resolve()));

    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  interface WsClient {
    ws: WebSocket;
    messages: any[];
    waitFor: (predicate: (msg: any) => boolean, timeoutMs?: number) => Promise<any>;
  }

  function createClient(sessionId: string): Promise<WsClient> {
    return new Promise((resolve, reject) => {
      const messages: any[] = [];
      const pending: Array<{ predicate: (m: any) => boolean; resolve: (m: any) => void; reject: (e: Error) => void; timer: ReturnType<typeof setTimeout> }> = [];

      const ws = new WebSocket(`ws://127.0.0.1:${port}/api/term/${sessionId}`);

      ws.on('message', (raw: Buffer) => {
        const msg = JSON.parse(raw.toString());
        messages.push(msg);
        for (let i = pending.length - 1; i >= 0; i--) {
          if (pending[i].predicate(msg)) {
            clearTimeout(pending[i].timer);
            pending[i].resolve(msg);
            pending.splice(i, 1);
          }
        }
      });

      ws.on('open', () => {
        resolve({
          ws,
          messages,
          waitFor(predicate: (m: any) => boolean, timeoutMs = 3000): Promise<any> {
            const existing = messages.find(predicate);
            if (existing) return Promise.resolve(existing);
            return new Promise((res, rej) => {
              const timer = setTimeout(() => rej(new Error('Timed out waiting for message')), timeoutMs);
              pending.push({ predicate, resolve: res, reject: rej, timer });
            });
          },
        });
      });

      ws.on('error', reject);
    });
  }

  it('establishes WebSocket connection and receives initial status', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const client = await createClient(session.id);

    const msg = await client.waitFor(m => m.type === 'status');
    expect(msg.session_id).toBe(session.id);
    expect(msg.viewer_id).toBeDefined();
    client.ws.close();
  });

  it('rejects WebSocket for nonexistent session', async () => {
    const ws = new WebSocket(`ws://127.0.0.1:${port}/api/term/nonexistent`);
    const closePromise = new Promise<number>((resolve) => {
      ws.on('close', (code: number) => resolve(code));
    });
    const code = await closePromise;
    expect(code).toBe(1008);
  });

  it('rejects WebSocket for invalid path', async () => {
    const ws = new WebSocket(`ws://127.0.0.1:${port}/api/health`);
    await new Promise<void>((resolve) => {
      ws.on('error', () => resolve());
      ws.on('close', () => resolve());
    });
    expect(ws.readyState).not.toBe(WebSocket.OPEN);
  });

  it('forwards PTY output through WebSocket', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const pty = transportLauncher.getHandle(session.id);
    expect(pty).toBeDefined();

    const outputPromise = client.waitFor(m => m.type === 'output' && m.data === 'hello world');
    (pty as unknown as EventEmitter).emit('data', 'hello world');
    const msg = await outputPromise;

    expect(msg.data).toBe('hello world');
    client.ws.close();
  });

  it('transitions state to connected on first PTY data', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    expect(session.state).toBe('connecting');

    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const pty = transportLauncher.getHandle(session.id);
    const statusPromise = client.waitFor(m => m.type === 'status' && m.state === 'connected');
    (pty as unknown as EventEmitter).emit('data', 'login banner');
    const msg = await statusPromise;

    expect(msg.state).toBe('connected');
    expect(sessionBroker.get(session.id)!.state).toBe('connected');
    client.ws.close();
  });

  it('sends scrollback to late-joining viewers', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const pty = transportLauncher.getHandle(session.id);

    // Emit data before any viewer connects
    (pty as unknown as EventEmitter).emit('data', 'line 1\r\n');
    (pty as unknown as EventEmitter).emit('data', 'line 2\r\n');

    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const outputMsg = await client.waitFor(m => m.type === 'output');
    expect(outputMsg.data).toContain('line 1');
    expect(outputMsg.data).toContain('line 2');
    client.ws.close();
  });

  it('delivers input from WebSocket to PTY', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const pty = transportLauncher.getHandle(session.id);
    const writeSpy = jest.spyOn(pty!, 'write');

    client.ws.send(JSON.stringify({ type: 'input', data: 'ls\n' }));
    await new Promise(r => setTimeout(r, 100));

    expect(writeSpy).toHaveBeenCalledWith('ls\n');
    client.ws.close();
  });

  it('handles resize messages', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const pty = transportLauncher.getHandle(session.id);
    const resizeSpy = jest.spyOn(pty!, 'resize');

    client.ws.send(JSON.stringify({ type: 'resize', cols: 120, rows: 40 }));
    await new Promise(r => setTimeout(r, 100));

    expect(resizeSpy).toHaveBeenCalledWith(120, 40);
    client.ws.close();
  });

  it('broadcasts PTY exit as disconnected status', async () => {
    const session = await sessionBroker.create({ username: 'user', hostname: 'test.example.com' });
    const client = await createClient(session.id);
    await client.waitFor(m => m.type === 'status');

    const pty = transportLauncher.getHandle(session.id);
    const disconnectPromise = client.waitFor(m => m.type === 'status' && m.state === 'disconnected');
    (pty as unknown as EventEmitter).emit('exit', { exitCode: 0 });
    const msg = await disconnectPromise;

    expect(msg.state).toBe('disconnected');
    client.ws.close();
  });
});
