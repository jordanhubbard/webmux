import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { EventEmitter } from 'events';

// ---------------------------------------------------------------------------
// Mock net.createConnection before any module is imported
// ---------------------------------------------------------------------------

const mockSocketInstance = new EventEmitter() as EventEmitter & {
  write: jest.Mock;
  destroy: jest.Mock;
  writable: boolean;
};
mockSocketInstance.write = jest.fn();
mockSocketInstance.destroy = jest.fn();
mockSocketInstance.writable = true;

jest.mock('net', () => ({
  createConnection: jest.fn(() => mockSocketInstance),
}));

// ---------------------------------------------------------------------------
// WebSocket readyState constants
// ---------------------------------------------------------------------------

const WS_OPEN = 1;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Build a minimal mock WebSocket object. */
function makeMockWs(readyState: number = WS_OPEN): EventEmitter & {
  send: jest.Mock;
  close: jest.Mock;
  readyState: number;
} {
  const ws = new EventEmitter() as EventEmitter & {
    send: jest.Mock;
    close: jest.Mock;
    readyState: number;
  };
  ws.send = jest.fn();
  ws.close = jest.fn();
  ws.readyState = readyState;
  return ws;
}

/** Build a minimal IncomingMessage-shaped object for the handler. */
function makeReq(url: string, host = 'localhost:8080'): { url: string; headers: { host: string } } {
  return { url, headers: { host } };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('VncHandler (TCP proxy)', () => {
  let tmpDir: string;
  let originalHome: string | undefined;

  // Loaded fresh per test to pick up env changes
  let setupVncWebSocket: (wss: any) => void;
  let vncBroker: any;
  let netModule: { createConnection: jest.Mock };

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-vnchandler-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    // Create required config files
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'), 'hosts: []\n');
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

    // Re-import after resetModules so persistence picks up the new WEBMUX_HOME
    ({ setupVncWebSocket } = require('@backend/websocket/vncHandler'));
    vncBroker = require('@backend/services/vncBroker').vncBroker;
    netModule = require('net');

    // Reset mock state between tests
    mockSocketInstance.removeAllListeners();
    mockSocketInstance.write.mockReset();
    mockSocketInstance.destroy.mockReset();
    mockSocketInstance.writable = true;
    (netModule.createConnection as jest.Mock).mockReset();
    (netModule.createConnection as jest.Mock).mockReturnValue(mockSocketInstance);
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  // Helper: trigger a "connection" event on a fake WSS
  function triggerConnection(ws: any, req: any): void {
    const fakeWss = new EventEmitter();
    setupVncWebSocket(fakeWss);
    fakeWss.emit('connection', ws, req);
  }

  // ---------------------------------------------------------------------------
  // Auth mode: local — JWT required
  // ---------------------------------------------------------------------------

  describe('auth mode: local', () => {
    beforeEach(() => {
      const configDir = path.join(tmpDir, 'config');
      fs.writeFileSync(
        path.join(configDir, 'auth.yaml'),
        'auth:\n  mode: local\n  users: []\n'
      );
      jest.resetModules();
      ({ setupVncWebSocket } = require('@backend/websocket/vncHandler'));
      vncBroker = require('@backend/services/vncBroker').vncBroker;
      netModule = require('net');
      (netModule.createConnection as jest.Mock).mockReturnValue(mockSocketInstance);
    });

    it('closes with 1008 when no token is provided', () => {
      const ws = makeMockWs();
      const req = makeReq('/api/vnc/ws/some-session-id');
      triggerConnection(ws, req);
      expect(ws.close).toHaveBeenCalledWith(1008, 'Unauthorized');
    });

    it('closes with 1008 when an invalid token is provided', () => {
      const ws = makeMockWs();
      const req = makeReq('/api/vnc/ws/some-session-id?token=bad.jwt.here');
      triggerConnection(ws, req);
      expect(ws.close).toHaveBeenCalledWith(1008, 'Unauthorized');
    });

    it('closes with 1008 for missing path segments', () => {
      const ws = makeMockWs();
      const req = makeReq('/api/vnc/ws/');
      triggerConnection(ws, req);
      // No token → auth failure fires first (or invalid path)
      expect(ws.close).toHaveBeenCalledWith(1008, expect.any(String));
    });
  });

  // ---------------------------------------------------------------------------
  // Auth mode: none — token optional
  // ---------------------------------------------------------------------------

  describe('auth mode: none', () => {
    beforeEach(() => {
      const configDir = path.join(tmpDir, 'config');
      fs.writeFileSync(
        path.join(configDir, 'auth.yaml'),
        'auth:\n  mode: none\n  users: []\n'
      );
      jest.resetModules();
      ({ setupVncWebSocket } = require('@backend/websocket/vncHandler'));
      vncBroker = require('@backend/services/vncBroker').vncBroker;
      netModule = require('net');
      (netModule.createConnection as jest.Mock).mockReturnValue(mockSocketInstance);
    });

    it('closes with 1008 for invalid path', () => {
      const ws = makeMockWs();
      const req = makeReq('/api/invalid-path');
      triggerConnection(ws, req);
      expect(ws.close).toHaveBeenCalledWith(1008, 'Invalid path');
    });

    it('closes with 1008 when session not found', () => {
      const ws = makeMockWs();
      const req = makeReq('/api/vnc/ws/nonexistent-session');
      triggerConnection(ws, req);
      expect(ws.close).toHaveBeenCalledWith(1008, 'Session not found');
    });

    it('closes with 1008 when session owner does not match authenticated user', async () => {
      // Create a session owned by 'alice'
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'alice');

      // A valid token for 'bob'
      const { signToken } = require('@backend/middleware/auth');
      const token = signToken('bob');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}?token=${token}`);
      triggerConnection(ws, req);
      expect(ws.close).toHaveBeenCalledWith(1008, 'Forbidden');
    });

    it('opens TCP connection and forwards data from TCP to ws.send', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      // Emit TCP data
      const chunk = Buffer.from('RFB 003.008\n');
      mockSocketInstance.emit('data', chunk);

      expect(ws.send).toHaveBeenCalledWith(chunk);
    });

    it('forwards WebSocket message event to socket.write', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      const data = Buffer.from('client handshake');
      ws.emit('message', data);

      expect(mockSocketInstance.write).toHaveBeenCalledWith(Buffer.from(data));
    });

    it('does not call socket.write when socket is not writable', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      mockSocketInstance.writable = false;
      ws.emit('message', Buffer.from('data'));

      expect(mockSocketInstance.write).not.toHaveBeenCalled();
    });

    it('TCP close event calls vncBroker.setState(disconnected) and ws.close(1001)', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');
      const setStateSpy = jest.spyOn(vncBroker, 'setState');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      mockSocketInstance.emit('close');

      expect(setStateSpy).toHaveBeenCalledWith(session.id, 'disconnected');
      expect(ws.close).toHaveBeenCalledWith(1001, 'TCP connection closed');
    });

    it('TCP error event calls vncBroker.setState(error) and ws.close(1011)', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');
      const setStateSpy = jest.spyOn(vncBroker, 'setState');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      mockSocketInstance.emit('error', new Error('ECONNREFUSED'));

      expect(setStateSpy).toHaveBeenCalledWith(session.id, 'error');
      expect(ws.close).toHaveBeenCalledWith(1011, 'TCP connection error');
    });

    it('WebSocket close event calls socket.destroy()', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      ws.emit('close');

      expect(mockSocketInstance.destroy).toHaveBeenCalled();
    });

    it('TCP connect event calls vncBroker.setState(connected)', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');
      const setStateSpy = jest.spyOn(vncBroker, 'setState');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      mockSocketInstance.emit('connect');

      expect(setStateSpy).toHaveBeenCalledWith(session.id, 'connected');
    });

    it('does not call ws.send when ws is not OPEN', async () => {
      await vncBroker.initialize();
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'anonymous');

      // ws.readyState is CLOSING (2) — not OPEN
      const ws = makeMockWs(2);
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      mockSocketInstance.emit('data', Buffer.from('hello'));

      expect(ws.send).not.toHaveBeenCalled();
    });

    it('closes with 1008 for invalid hostname (SSRF prevention)', async () => {
      await vncBroker.initialize();
      // hostname with spaces is invalid
      const session = await vncBroker.create({ hostname: 'invalid hostname!' }, 'anonymous');

      const ws = makeMockWs();
      const req = makeReq(`/api/vnc/ws/${session.id}`);
      triggerConnection(ws, req);

      expect(ws.close).toHaveBeenCalledWith(1008, 'Invalid hostname');
      // net.createConnection should NOT have been called
      expect(netModule.createConnection).not.toHaveBeenCalled();
    });
  });
});
