import { useState, useEffect, useCallback, useRef } from 'react';
import { Tile } from './Tile';
import { ConnectionDialog } from './ConnectionDialog';
import { api } from '../utils/api';
import type { Session, CreateSessionRequest } from '../types';

interface WorkspaceProps {
  fontSize: number;
  showAddDialog: boolean;
  onDialogClose: () => void;
}

const TILE_W = 660;
const TILE_H = 440;
const TILE_GAP = 16;

export function Workspace({ fontSize, showAddDialog, onDialogClose }: WorkspaceProps) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [splitPos, setSplitPos] = useState<{ row?: number; col?: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    api.getSessions()
      .then(setSessions)
      .catch(err => console.error('Failed to load sessions:', err))
      .finally(() => setLoading(false));
  }, []);

  const handleAddSession = useCallback(async (req: CreateSessionRequest) => {
    const session = await api.createSession(req);
    setSessions(prev => [...prev, session]);
    onDialogClose();
    setSplitPos(null);
  }, [onDialogClose]);

  const handleClose = useCallback((id: string) => {
    api.deleteSession(id).then(() => {
      setSessions(prev => prev.filter(s => s.id !== id));
    }).catch(err => console.error('Failed to delete session:', err));
  }, []);

  const handleSplitRight = useCallback((id: string) => {
    api.splitRight(id).then(pos => {
      setSplitPos(pos);
    }).catch(err => console.error('Split right error:', err));
  }, []);

  const handleSplitBelow = useCallback((id: string) => {
    api.splitBelow(id).then(pos => {
      setSplitPos(pos);
    }).catch(err => console.error('Split below error:', err));
  }, []);

  const handleReconnect = useCallback((id: string) => {
    api.reconnectSession(id).then(updated => {
      setSessions(prev => prev.map(s => s.id === id ? updated : s));
    }).catch(err => console.error('Reconnect error:', err));
  }, []);

  const maxRow = sessions.length > 0 ? Math.max(...sessions.map(s => s.row)) : 0;
  const rows: Session[][] = [];
  for (let r = 0; r <= maxRow; r++) {
    rows.push(sessions.filter(s => s.row === r).sort((a, b) => a.col - b.col));
  }

  const totalW = rows.reduce((max, row) => {
    const w = row.length * (TILE_W + TILE_GAP) + TILE_GAP;
    return Math.max(max, w);
  }, 400);
  const totalH = (maxRow + 1) * (TILE_H + TILE_GAP) + TILE_GAP;

  const isDialogOpen = showAddDialog || splitPos !== null;

  return (
    <div style={styles.workspaceOuter}>
      <div
        ref={containerRef}
        style={{
          ...styles.workspaceScroll,
          minWidth: totalW,
          minHeight: totalH,
        }}
      >
        {loading && (
          <div style={styles.loading}>Loading sessions…</div>
        )}

        {!loading && sessions.length === 0 && (
          <div style={styles.empty}>
            <div style={styles.emptyIcon}>▦</div>
            <div style={styles.emptyText}>No active sessions</div>
            <div style={styles.emptyHint}>Click <strong>+ New Session</strong> to get started</div>
          </div>
        )}

        {rows.map((row, rowIdx) => (
          <div key={rowIdx} style={styles.row}>
            {row.map(session => (
              <Tile
                key={session.id}
                session={session}
                fontSize={fontSize}
                onClose={handleClose}
                onSplitRight={handleSplitRight}
                onSplitBelow={handleSplitBelow}
                onReconnect={handleReconnect}
              />
            ))}
          </div>
        ))}
      </div>

      {isDialogOpen && (
        <ConnectionDialog
          onConnect={handleAddSession}
          onClose={() => { onDialogClose(); setSplitPos(null); }}
          suggestedRow={splitPos?.row}
          suggestedCol={splitPos?.col}
        />
      )}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  workspaceOuter: {
    flex: 1,
    overflow: 'auto',
    background: '#0d0d1a',
  },
  workspaceScroll: {
    padding: TILE_GAP,
    display: 'flex',
    flexDirection: 'column',
    gap: TILE_GAP,
  },
  row: {
    display: 'flex',
    gap: TILE_GAP,
    alignItems: 'flex-start',
  },
  loading: {
    color: '#888',
    fontSize: 14,
    padding: 32,
    textAlign: 'center',
  },
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    paddingTop: 80,
    gap: 12,
  },
  emptyIcon: {
    fontSize: 48,
    color: '#333366',
  },
  emptyText: {
    fontSize: 18,
    color: '#555555',
    fontWeight: 600,
  },
  emptyHint: {
    fontSize: 13,
    color: '#444444',
  },
};
