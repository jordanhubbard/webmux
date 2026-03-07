import { EventEmitter } from 'events';
import WebSocket from 'ws';

interface ViewerInfo {
  viewerId: string;
  sessionId: string;
  ws: WebSocket;
  has_focus: boolean;
  joined_at: string;
}

export class PresenceService extends EventEmitter {
  private viewers = new Map<string, ViewerInfo>();
  // sessionId -> Set<viewerId>
  private sessionViewers = new Map<string, Set<string>>();
  // sessionId -> focused viewerId
  private focusOwners = new Map<string, string>();

  join(viewerId: string, sessionId: string, ws: WebSocket): void {
    this.viewers.set(viewerId, {
      viewerId,
      sessionId,
      ws,
      has_focus: false,
      joined_at: new Date().toISOString(),
    });

    if (!this.sessionViewers.has(sessionId)) {
      this.sessionViewers.set(sessionId, new Set());
    }
    this.sessionViewers.get(sessionId)!.add(viewerId);

    // Auto-grant focus to the first viewer (or if no one holds focus)
    if (!this.focusOwners.has(sessionId)) {
      this.focusOwners.set(sessionId, viewerId);
      this.viewers.get(viewerId)!.has_focus = true;
    }

    this.broadcastToSession(sessionId, {
      type: 'viewer_join',
      session_id: sessionId,
      viewer_id: viewerId,
      viewer_count: this.getViewerCount(sessionId),
      focus_owner: this.getFocusOwner(sessionId),
    });
  }

  leave(viewerId: string): void {
    const viewer = this.viewers.get(viewerId);
    if (!viewer) return;

    const { sessionId } = viewer;
    this.viewers.delete(viewerId);

    const sv = this.sessionViewers.get(sessionId);
    if (sv) {
      sv.delete(viewerId);
      if (sv.size === 0) this.sessionViewers.delete(sessionId);
    }

    // Release focus if this viewer held it
    if (this.focusOwners.get(sessionId) === viewerId) {
      this.focusOwners.delete(sessionId);
    }

    this.broadcastToSession(sessionId, {
      type: 'viewer_leave',
      session_id: sessionId,
      viewer_id: viewerId,
      viewer_count: this.getViewerCount(sessionId),
      focus_owner: this.getFocusOwner(sessionId),
    });
  }

  takeFocus(viewerId: string, sessionId: string): void {
    this.focusOwners.set(sessionId, viewerId);
    const viewer = this.viewers.get(viewerId);
    if (viewer) viewer.has_focus = true;

    // Remove focus from other viewers on this session
    const sv = this.sessionViewers.get(sessionId);
    if (sv) {
      sv.forEach(vid => {
        if (vid !== viewerId) {
          const v = this.viewers.get(vid);
          if (v) v.has_focus = false;
        }
      });
    }

    this.broadcastToSession(sessionId, {
      type: 'focus',
      session_id: sessionId,
      focus_owner: viewerId,
      viewer_count: this.getViewerCount(sessionId),
    });
  }

  getViewerCount(sessionId: string): number {
    return this.sessionViewers.get(sessionId)?.size || 0;
  }

  getFocusOwner(sessionId: string): string | undefined {
    return this.focusOwners.get(sessionId);
  }

  hasFocus(viewerId: string, sessionId: string): boolean {
    return this.focusOwners.get(sessionId) === viewerId;
  }

  getViewersForSession(sessionId: string): ViewerInfo[] {
    const sv = this.sessionViewers.get(sessionId);
    if (!sv) return [];
    return Array.from(sv).map(vid => this.viewers.get(vid)).filter(Boolean) as ViewerInfo[];
  }

  broadcastToSession(sessionId: string, message: Record<string, unknown>): void {
    const sv = this.sessionViewers.get(sessionId);
    if (!sv) return;
    const data = JSON.stringify(message);
    sv.forEach(vid => {
      const viewer = this.viewers.get(vid);
      if (viewer?.ws.readyState === WebSocket.OPEN) {
        viewer.ws.send(data);
      }
    });
  }

  sendToViewer(viewerId: string, message: Record<string, unknown>): void {
    const viewer = this.viewers.get(viewerId);
    if (viewer?.ws.readyState === WebSocket.OPEN) {
      viewer.ws.send(JSON.stringify(message));
    }
  }
}

export const presenceService = new PresenceService();
