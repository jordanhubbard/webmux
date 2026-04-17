import net from 'net';
import { WebSocketServer, WebSocket } from 'ws';
import { IncomingMessage } from 'http';
import { rdpBroker } from '../services/rdpBroker';
import { TransportLauncher } from '../services/transportLauncher';
import { verifyToken } from '../middleware/auth';
import { persistence } from '../services/persistenceManager';

const DEFAULT_GUACD_HOST = '127.0.0.1';
const DEFAULT_GUACD_PORT = 4822;

// Encodes a Guacamole protocol instruction: "LEN.VALUE,LEN.VALUE,...;"
function encodeGuac(opcode: string, ...args: string[]): string {
  return [opcode, ...args].map(s => `${s.length}.${s}`).join(',') + ';';
}

// Parses one complete Guacamole instruction (must end with ';')
function parseGuac(instruction: string): { opcode: string; args: string[] } | null {
  const parts: string[] = [];
  let i = 0;
  const s = instruction.endsWith(';') ? instruction.slice(0, -1) : instruction;
  while (i < s.length) {
    const dot = s.indexOf('.', i);
    if (dot === -1) break;
    const len = parseInt(s.slice(i, dot), 10);
    if (isNaN(len)) break;
    parts.push(s.slice(dot + 1, dot + 1 + len));
    i = dot + 1 + len;
    if (s[i] === ',') i++;
  }
  if (parts.length === 0) return null;
  return { opcode: parts[0], args: parts.slice(1) };
}

// Returns the value for a given RDP connection parameter name
function rdpParamValue(
  param: string,
  hostname: string,
  port: number,
  username: string,
  password: string,
  domain: string,
): string {
  switch (param) {
    case 'hostname': return hostname;
    case 'port': return String(port);
    case 'username': return username;
    case 'password': return password;
    case 'domain': return domain;
    case 'width': return '1024';
    case 'height': return '768';
    case 'dpi': return '96';
    case 'color-depth': return '24';
    case 'security': return 'any';
    case 'ignore-cert': return 'true';
    case 'client-name': return 'webmux';
    case 'resize-method': return 'display-update';
    case 'enable-font-smoothing': return 'true';
    case 'enable-desktop-composition': return 'true';
    case 'enable-full-window-drag': return 'false';
    case 'enable-menu-animations': return 'false';
    case 'disable-bitmap-caching': return 'false';
    case 'disable-offscreen-caching': return 'false';
    case 'disable-glyph-caching': return 'false';
    case 'disable-audio': return 'false';
    case 'enable-drive': return 'false';
    case 'enable-printing': return 'false';
    case 'sftp-enable': return 'false';
    case 'wol-send-packet': return 'false';
    case 'normalize-clipboard': return 'preserve';
    case 'enable-touch': return 'false';
    case 'recording-exclude-output': return 'false';
    case 'recording-exclude-mouse': return 'false';
    case 'recording-include-keys': return 'false';
    case 'create-recording-path': return 'false';
    case 'create-drive-path': return 'false';
    default: return '';
  }
}

export function setupRdpWebSocket(wss: WebSocketServer): void {
  wss.on('connection', (ws: WebSocket, req: IncomingMessage) => {
    const match = req.url?.match(/\/api\/rdp\/ws\/([^/?]+)/);
    if (!match) { ws.close(1008, 'Invalid path'); return; }
    const sessionId = match[1];

    const url = new URL(req.url!, `http://${req.headers.host}`);
    const token = url.searchParams.get('token') || undefined;

    let owner = 'anonymous';
    let authRequired = true;
    try {
      const authConfig = persistence.loadAuth();
      if (authConfig.auth.mode === 'none') authRequired = false;
    } catch { /* default to requiring auth */ }

    if (authRequired) {
      if (!token) { ws.close(1008, 'Unauthorized'); return; }
      try {
        const payload = verifyToken(token);
        owner = payload.sub;
      } catch { ws.close(1008, 'Unauthorized'); return; }
    } else if (token) {
      try { owner = verifyToken(token).sub; } catch { /* optional token */ }
    }

    const session = rdpBroker.get(sessionId);
    if (!session) { ws.close(1008, 'Session not found'); return; }
    if (session.owner !== owner) { ws.close(1008, 'Forbidden'); return; }

    try {
      TransportLauncher.validateHostname(session.hostname);
    } catch { ws.close(1008, 'Invalid hostname'); return; }

    const password = rdpBroker.getPassword(sessionId) || '';

    let guacdHost = DEFAULT_GUACD_HOST;
    let guacdPort = DEFAULT_GUACD_PORT;
    try {
      const appConfig = persistence.loadApp();
      if (appConfig.app.guacd) {
        guacdHost = appConfig.app.guacd.host || DEFAULT_GUACD_HOST;
        guacdPort = appConfig.app.guacd.port || DEFAULT_GUACD_PORT;
      }
    } catch { /* use defaults */ }

    const socket = net.createConnection(guacdPort, guacdHost);
    let handshakeDone = false;
    let tcpBuf = '';

    socket.on('connect', () => {
      socket.write(encodeGuac('select', 'rdp'));
    });

    socket.on('data', (chunk: Buffer) => {
      if (handshakeDone) {
        if (ws.readyState === WebSocket.OPEN) ws.send(chunk.toString('utf-8'));
        return;
      }

      tcpBuf += chunk.toString('utf-8');

      // Process all complete instructions in buffer
      while (true) {
        const semi = tcpBuf.indexOf(';');
        if (semi === -1) break;

        const instruction = tcpBuf.slice(0, semi + 1);
        tcpBuf = tcpBuf.slice(semi + 1);

        const parsed = parseGuac(instruction);
        if (!parsed) continue;

        if (parsed.opcode === 'args') {
          // guacd is telling us what parameters it needs; respond with connect
          const paramValues = parsed.args.map(p =>
            rdpParamValue(p, session.hostname, session.rdp_port,
              session.rdp_username, password, session.rdp_domain)
          );
          socket.write(encodeGuac('connect', ...paramValues));

        } else if (parsed.opcode === 'ready') {
          handshakeDone = true;
          rdpBroker.setState(sessionId, 'connected');
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(instruction);
            if (tcpBuf) { ws.send(tcpBuf); tcpBuf = ''; }
          }

        } else if (parsed.opcode === 'error') {
          console.error(`RDP guacd error for session ${sessionId}:`, parsed.args);
          rdpBroker.setState(sessionId, 'error');
          socket.destroy();
          if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
            ws.close(1011, 'guacd error');
          }
          return;
        }
      }
    });

    socket.on('close', () => {
      rdpBroker.setState(sessionId, 'disconnected');
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close(1001, 'guacd connection closed');
      }
    });

    socket.on('error', (err: Error) => {
      console.error(`RDP guacd error for session ${sessionId}:`, err.message);
      rdpBroker.setState(sessionId, 'error');
      socket.destroy();
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close(1011, 'guacd connection error');
      }
    });

    // Browser → guacd (text passthrough, only after handshake)
    ws.on('message', (data: Buffer | string) => {
      if (handshakeDone && socket.writable) {
        socket.write(typeof data === 'string' ? data : (data as Buffer).toString('utf-8'));
      }
    });

    ws.on('close', () => { socket.destroy(); });

    ws.on('error', (err: Error) => {
      console.error(`RDP WebSocket error for session ${sessionId}:`, err.message);
      socket.destroy();
    });
  });
}
