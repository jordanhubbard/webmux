import net from 'net';
import { WebSocketServer, WebSocket } from 'ws';
import { IncomingMessage } from 'http';
import { vncBroker } from '../services/vncBroker';
import { TransportLauncher } from '../services/transportLauncher';
import { verifyToken } from '../middleware/auth';
import { persistence } from '../services/persistenceManager';

export function setupVncWebSocket(wss: WebSocketServer): void {
  wss.on('connection', (ws: WebSocket, req: IncomingMessage) => {
    // Extract session ID from path /api/vnc/ws/:id
    const match = req.url?.match(/\/api\/vnc\/ws\/([^/?]+)/);
    if (!match) {
      ws.close(1008, 'Invalid path');
      return;
    }
    const sessionId = match[1];

    // Authenticate via query param token
    const url = new URL(req.url!, `http://${req.headers.host}`);
    const token = url.searchParams.get('token') || undefined;

    let owner = 'anonymous';
    let authRequired = true;
    try {
      const authConfig = persistence.loadAuth();
      if (authConfig.auth.mode === 'none') {
        authRequired = false;
      }
    } catch {
      // Default to requiring auth if config can't be loaded
    }

    if (authRequired) {
      if (!token) {
        ws.close(1008, 'Unauthorized');
        return;
      }
      try {
        const payload = verifyToken(token);
        owner = payload.sub;
      } catch {
        ws.close(1008, 'Unauthorized');
        return;
      }
    } else if (token) {
      try {
        const payload = verifyToken(token);
        owner = payload.sub;
      } catch {
        // auth mode is 'none', token is optional — ignore invalid token
      }
    }

    // Look up VNC session
    const session = vncBroker.get(sessionId);
    if (!session) {
      ws.close(1008, 'Session not found');
      return;
    }

    // Verify ownership
    if (session.owner !== owner) {
      ws.close(1008, 'Forbidden');
      return;
    }

    // Validate hostname (SSRF prevention)
    try {
      TransportLauncher.validateHostname(session.hostname);
    } catch (err) {
      ws.close(1008, 'Invalid hostname');
      return;
    }

    // Open TCP connection to VNC server
    const socket = net.createConnection(session.vnc_port, session.hostname);

    // TCP → WebSocket (binary)
    socket.on('data', (chunk: Buffer) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(chunk);
      }
    });

    // TCP connected
    socket.on('connect', () => {
      vncBroker.setState(sessionId, 'connected');
    });

    // TCP closed
    socket.on('close', () => {
      vncBroker.setState(sessionId, 'disconnected');
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close(1001, 'TCP connection closed');
      }
    });

    // TCP error
    socket.on('error', (err: Error) => {
      console.error(`VNC TCP error for session ${sessionId}:`, err);
      vncBroker.setState(sessionId, 'error');
      socket.destroy();
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close(1011, 'TCP connection error');
      }
    });

    // WebSocket → TCP (binary)
    ws.on('message', (data: Buffer | ArrayBuffer | Buffer[]) => {
      if (socket.writable) {
        const buf = Buffer.isBuffer(data)
          ? data
          : Array.isArray(data)
            ? Buffer.concat(data)
            : Buffer.from(data as ArrayBuffer);
        socket.write(buf);
      }
    });

    // WebSocket closed → destroy TCP socket
    ws.on('close', () => {
      socket.destroy();
    });

    ws.on('error', (err: Error) => {
      console.error(`VNC WebSocket error for session ${sessionId}:`, err);
      socket.destroy();
    });
  });
}
