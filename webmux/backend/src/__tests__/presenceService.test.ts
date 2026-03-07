import { PresenceService } from '../services/presenceService';
import WebSocket from 'ws';

function mockWs(readyState: number = WebSocket.OPEN): WebSocket {
  return {
    readyState,
    send: jest.fn(),
    close: jest.fn(),
  } as unknown as WebSocket;
}

describe('PresenceService', () => {
  let ps: PresenceService;

  beforeEach(() => {
    ps = new PresenceService();
  });

  it('tracks viewer count after join', () => {
    const ws = mockWs();
    ps.join('v1', 'sess1', ws);
    expect(ps.getViewerCount('sess1')).toBe(1);
  });

  it('broadcasts viewer_join to session viewers', () => {
    const ws1 = mockWs();
    const ws2 = mockWs();
    ps.join('v1', 'sess1', ws1);
    ps.join('v2', 'sess1', ws2);

    // ws1 should have received the viewer_join for v2
    const calls = (ws1.send as jest.Mock).mock.calls;
    const msgs = calls.map((c: [string]) => JSON.parse(c[0]));
    const joinMsg = msgs.find((m: Record<string, unknown>) => m.type === 'viewer_join' && m.viewer_id === 'v2');
    expect(joinMsg).toBeDefined();
    expect(joinMsg.viewer_count).toBe(2);
  });

  it('decrements viewer count after leave', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.join('v2', 'sess1', mockWs());
    ps.leave('v1');
    expect(ps.getViewerCount('sess1')).toBe(1);
  });

  it('cleans up session viewers set when last viewer leaves', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.leave('v1');
    expect(ps.getViewerCount('sess1')).toBe(0);
  });

  it('leave is safe for unknown viewer', () => {
    expect(() => ps.leave('nonexistent')).not.toThrow();
  });

  it('takeFocus sets focus owner', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.join('v2', 'sess1', mockWs());
    ps.takeFocus('v1', 'sess1');
    expect(ps.getFocusOwner('sess1')).toBe('v1');
    expect(ps.hasFocus('v1', 'sess1')).toBe(true);
    expect(ps.hasFocus('v2', 'sess1')).toBe(false);
  });

  it('takeFocus transfers focus between viewers', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.join('v2', 'sess1', mockWs());
    ps.takeFocus('v1', 'sess1');
    ps.takeFocus('v2', 'sess1');
    expect(ps.getFocusOwner('sess1')).toBe('v2');
    expect(ps.hasFocus('v1', 'sess1')).toBe(false);
    expect(ps.hasFocus('v2', 'sess1')).toBe(true);
  });

  it('releases focus when focus owner leaves', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.takeFocus('v1', 'sess1');
    ps.leave('v1');
    expect(ps.getFocusOwner('sess1')).toBeUndefined();
  });

  it('broadcastToSession sends to all open sockets', () => {
    const ws1 = mockWs();
    const ws2 = mockWs();
    const wsClosed = mockWs(3); // WebSocket.CLOSED = 3
    ps.join('v1', 'sess1', ws1);
    ps.join('v2', 'sess1', ws2);
    ps.join('v3', 'sess1', wsClosed);

    // Clear join broadcast calls
    (ws1.send as jest.Mock).mockClear();
    (ws2.send as jest.Mock).mockClear();

    ps.broadcastToSession('sess1', { type: 'test' });
    expect(ws1.send).toHaveBeenCalledTimes(1);
    expect(ws2.send).toHaveBeenCalledTimes(1);
    expect(wsClosed.send).not.toHaveBeenCalled();
  });

  it('broadcastToSession does nothing for unknown session', () => {
    expect(() => ps.broadcastToSession('unknown', { type: 'test' })).not.toThrow();
  });

  it('sendToViewer sends only to the specified viewer', () => {
    const ws1 = mockWs();
    const ws2 = mockWs();
    ps.join('v1', 'sess1', ws1);
    ps.join('v2', 'sess1', ws2);
    (ws1.send as jest.Mock).mockClear();
    (ws2.send as jest.Mock).mockClear();

    ps.sendToViewer('v1', { type: 'private' });
    expect(ws1.send).toHaveBeenCalledTimes(1);
    expect(ws2.send).not.toHaveBeenCalled();
  });

  it('getViewersForSession returns viewer info', () => {
    ps.join('v1', 'sess1', mockWs());
    ps.join('v2', 'sess1', mockWs());
    const viewers = ps.getViewersForSession('sess1');
    expect(viewers).toHaveLength(2);
    expect(viewers.map(v => v.viewerId).sort()).toEqual(['v1', 'v2']);
  });

  it('getViewersForSession returns empty for unknown session', () => {
    expect(ps.getViewersForSession('unknown')).toEqual([]);
  });
});
