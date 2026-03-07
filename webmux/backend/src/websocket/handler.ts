import WebSocket, { WebSocketServer } from 'ws';
import { IncomingMessage } from 'http';
import { v4 as uuidv4 } from 'uuid';
import { sessionBroker } from '../services/sessionBroker';
import { presenceService } from '../services/presenceService';
import { requireAuthWs } from '../middleware/auth';
import { WebSocketMessage } from '../types';

export function setupWebSocket(wss: WebSocketServer): void {
  wss.on('connection', (ws: WebSocket, req: IncomingMessage) => {
    // Extract session ID from path /api/term/:id
    const match = req.url?.match(/\/api\/term\/([^/?]+)/);
    if (!match) {
      ws.close(1008, 'Invalid path');
      return;
    }
    const sessionId = match[1];

    // Authenticate via query param token
    const url = new URL(req.url!, `http://${req.headers.host}`);
    const token = url.searchParams.get('token') || undefined;

    if (!requireAuthWs(token)) {
      ws.send(JSON.stringify({ type: 'error', message: 'Unauthorized' }));
      ws.close(1008, 'Unauthorized');
      return;
    }

    const viewerId = uuidv4();

    // Join the session
    const session = sessionBroker.get(sessionId);
    if (!session) {
      ws.send(JSON.stringify({ type: 'error', message: 'Session not found' }));
      ws.close(1008, 'Session not found');
      return;
    }

    presenceService.join(viewerId, sessionId, ws);

    // Send current session state
    ws.send(JSON.stringify({
      type: 'status',
      session_id: sessionId,
      state: session.state,
      viewer_id: viewerId,
    }));

    ws.on('message', (raw: Buffer) => {
      let msg: WebSocketMessage;
      try {
        msg = JSON.parse(raw.toString()) as WebSocketMessage;
      } catch {
        return;
      }

      switch (msg.type) {
        case 'input':
          if (presenceService.hasFocus(viewerId, sessionId) && msg.data) {
            sessionBroker.sendInput(sessionId, msg.data);
          }
          break;

        case 'resize':
          if (msg.cols && msg.rows) {
            sessionBroker.resize(sessionId, msg.cols, msg.rows);
          }
          break;

        case 'focus':
          presenceService.takeFocus(viewerId, sessionId);
          break;

        default:
          break;
      }
    });

    ws.on('close', () => {
      presenceService.leave(viewerId);
    });

    ws.on('error', (err: Error) => {
      console.error(`WebSocket error for viewer ${viewerId}:`, err);
      presenceService.leave(viewerId);
    });
  });
}
