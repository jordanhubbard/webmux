import { useState, useEffect, useCallback } from 'react';
import { Tile } from './Tile';
import { ConnectionDialog } from './ConnectionDialog';
import { api } from '../utils/api';
import type { Session, CreateSessionRequest } from '../types';

interface WorkspaceProps {
  fontSize: number;
  termCols: number;
  termRows: number;
}

const GAP = 8;
const CHAR_W_RATIO = 0.602;
const CHAR_H_RATIO = 1.2;
const CHROME_H = 30;
const TILE_PADDING = 24;

function tilePixelSize(cols: number, rows: number, fontSize: number) {
  const w = Math.ceil(cols * fontSize * CHAR_W_RATIO) + TILE_PADDING;
  const h = Math.ceil(rows * fontSize * CHAR_H_RATIO) + CHROME_H + TILE_PADDING;
  return { w, h };
}

function getAddPositions(sessions: Session[]): { row: number; col: number }[] {
  if (sessions.length === 0) return [{ row: 0, col: 0 }];

  const occupied = new Set(sessions.map(s => `${s.row},${s.col}`));
  const positions: { row: number; col: number }[] = [];
  const seen = new Set<string>();

  for (const s of sessions) {
    const right = `${s.row},${s.col + 1}`;
    if (!occupied.has(right) && !seen.has(right)) {
      positions.push({ row: s.row, col: s.col + 1 });
      seen.add(right);
    }
    const below = `${s.row + 1},${s.col}`;
    if (!occupied.has(below) && !seen.has(below)) {
      positions.push({ row: s.row + 1, col: s.col });
      seen.add(below);
    }
  }

  return positions;
}

function AddCell({ row, col, isEmpty, onClick }: {
  row: number;
  col: number;
  isEmpty: boolean;
  onClick: () => void;
}) {
  const [hovered, setHovered] = useState(false);

  return (
    <div
      style={{
        gridColumn: col + 1,
        gridRow: row + 1,
        border: `2px dashed ${hovered ? '#7c6af7' : '#1e1e3a'}`,
        borderRadius: 6,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        transition: 'border-color 0.2s, background 0.2s',
        background: hovered ? 'rgba(124, 106, 247, 0.06)' : 'transparent',
        gap: 12,
        minHeight: 0,
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={onClick}
      data-testid={`add-cell-${row}-${col}`}
    >
      <span style={{
        fontSize: isEmpty ? 64 : 36,
        fontWeight: 300,
        color: hovered ? '#7c6af7' : '#2a2a4a',
        transition: 'color 0.2s',
        lineHeight: 1,
        userSelect: 'none',
      }}>+</span>
      {isEmpty && (
        <span style={{
          fontSize: 14,
          color: hovered ? '#7c6af7' : '#3a3a5a',
          transition: 'color 0.2s',
          userSelect: 'none',
        }}>Click to add a session</span>
      )}
    </div>
  );
}

export function Workspace({ fontSize, termCols, termRows }: WorkspaceProps) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogPos, setDialogPos] = useState<{ row: number; col: number } | null>(null);

  useEffect(() => {
    api.getSessions()
      .then(setSessions)
      .catch(err => console.error('Failed to load sessions:', err))
      .finally(() => setLoading(false));
  }, []);

  const handleAddSession = useCallback(async (req: CreateSessionRequest) => {
    const session = await api.createSession(req);
    setSessions(prev => [...prev, session]);
    setDialogPos(null);
  }, []);

  const handleClose = useCallback((id: string) => {
    api.deleteSession(id).then(() => {
      setSessions(prev => prev.filter(s => s.id !== id));
    }).catch(err => console.error('Failed to delete session:', err));
  }, []);

  const handleReconnect = useCallback((id: string) => {
    api.reconnectSession(id).then(updated => {
      setSessions(prev => prev.map(s => s.id === id ? updated : s));
    }).catch(err => console.error('Reconnect error:', err));
  }, []);

  const addPositions = getAddPositions(sessions);

  const allPositions = [
    ...sessions.map(s => ({ row: s.row, col: s.col })),
    ...addPositions,
  ];
  const numCols = allPositions.length > 0 ? Math.max(...allPositions.map(p => p.col)) + 1 : 1;
  const numRows = allPositions.length > 0 ? Math.max(...allPositions.map(p => p.row)) + 1 : 1;
  const tile = tilePixelSize(termCols, termRows, fontSize);

  if (loading) {
    return (
      <div style={styles.outer}>
        <div style={styles.loading}>Loading sessions\u2026</div>
      </div>
    );
  }

  return (
    <div style={styles.outer}>
      <div style={{
        ...styles.grid,
        gridTemplateColumns: `repeat(${numCols}, ${tile.w}px)`,
        gridTemplateRows: `repeat(${numRows}, ${tile.h}px)`,
      }}>
        {sessions.map(session => (
          <div
            key={session.id}
            style={{
              gridColumn: session.col + 1,
              gridRow: session.row + 1,
              minHeight: 0,
              minWidth: 0,
              display: 'flex',
            }}
          >
            <Tile
              session={session}
              fontSize={fontSize}
              onClose={handleClose}
              onReconnect={handleReconnect}
            />
          </div>
        ))}

        {addPositions.map(pos => (
          <AddCell
            key={`add-${pos.row}-${pos.col}`}
            row={pos.row}
            col={pos.col}
            isEmpty={sessions.length === 0}
            onClick={() => setDialogPos(pos)}
          />
        ))}
      </div>

      {dialogPos && (
        <ConnectionDialog
          onConnect={handleAddSession}
          onClose={() => setDialogPos(null)}
          suggestedRow={dialogPos.row}
          suggestedCol={dialogPos.col}
        />
      )}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  outer: {
    flex: 1,
    overflow: 'auto',
    background: '#0d0d1a',
  },
  grid: {
    display: 'grid',
    gap: GAP,
    padding: GAP,
    minHeight: '100%',
    boxSizing: 'border-box',
  },
  loading: {
    color: '#888',
    fontSize: 14,
    padding: 32,
    textAlign: 'center',
  },
};
