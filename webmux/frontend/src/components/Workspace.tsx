import { useState, useEffect, useCallback, useRef } from 'react';
import { Tile } from './Tile';
import { ConnectionDialog } from './ConnectionDialog';
import { AISidebar } from './AISidebar';
import { api } from '../utils/api';
import type { Session, CreateSessionRequest } from '../types';

interface WorkspaceProps {
  fontSize: number;
  termCols: number;
  termRows: number;
  execCommand?: string;
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

export function Workspace({ fontSize, termCols, termRows, execCommand }: WorkspaceProps) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogPos, setDialogPos] = useState<{ row: number; col: number } | null>(null);
  const [aiOpen, setAiOpen] = useState(false);
  // Ref to get terminal scrollback for AI context
  const termContextRef = useRef<() => string>(() => '');

  // Drag state
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ row: number; col: number } | null>(null);
  const [ghostPos, setGhostPos] = useState({ x: 0, y: 0 });
  const gridRef = useRef<HTMLDivElement>(null);

  // Refs for use in event handlers (avoid stale closures)
  const sessionsRef = useRef<Session[]>([]);
  const draggingIdRef = useRef<string | null>(null);
  const dropTargetRef = useRef<{ row: number; col: number } | null>(null);
  useEffect(() => { sessionsRef.current = sessions; }, [sessions]);
  useEffect(() => { draggingIdRef.current = draggingId; }, [draggingId]);
  useEffect(() => {
    dropTargetRef.current = dropTarget;
  }, [dropTarget]);

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
    api.deleteSession(id)
      .then(() => api.getSessions())
      .then(setSessions)
      .catch(err => console.error('Failed to delete session:', err));
  }, []);

  const handleReconnect = useCallback((id: string) => {
    api.reconnectSession(id).then(updated => {
      setSessions(prev => prev.map(s => s.id === id ? updated : s));
    }).catch(err => console.error('Reconnect error:', err));
  }, []);

  const handleRename = useCallback((id: string, title: string) => {
    setSessions(prev => prev.map(s => s.id === id ? { ...s, title } : s));
    api.renameSession(id, title).catch(err => {
      console.error('Rename error:', err);
      api.getSessions().then(setSessions);
    });
  }, []);

  const tile = tilePixelSize(termCols, termRows, fontSize);

  const getGridCell = useCallback((clientX: number, clientY: number): { row: number; col: number } | null => {
    if (!gridRef.current) return null;
    const rect = gridRef.current.getBoundingClientRect();
    const x = clientX - rect.left - GAP;
    const y = clientY - rect.top - GAP;
    if (x < 0 || y < 0) return null;
    const col = Math.floor(x / (tile.w + GAP));
    const row = Math.floor(y / (tile.h + GAP));
    if (col < 0 || row < 0) return null;
    return { row, col };
  }, [tile.w, tile.h]);

  const handleTitleMouseDown = useCallback((sessionId: string, e: React.MouseEvent) => {
    e.preventDefault();
    setDraggingId(sessionId);
    draggingIdRef.current = sessionId;
    setGhostPos({ x: e.clientX, y: e.clientY });
  }, []);

  // Document-level drag handlers
  useEffect(() => {
    if (!draggingId) return;

    const onMouseMove = (e: MouseEvent) => {
      setGhostPos({ x: e.clientX, y: e.clientY });
      const cell = getGridCell(e.clientX, e.clientY);
      if (cell) {
        const sessionAtCell = sessionsRef.current.find(s => s.row === cell.row && s.col === cell.col);
        const isSelf = sessionAtCell && sessionAtCell.id === draggingIdRef.current;
        const newTarget = isSelf ? null : cell;
        setDropTarget(newTarget);
        dropTargetRef.current = newTarget;
      } else {
        setDropTarget(null);
        dropTargetRef.current = null;
      }
    };

    const onMouseUp = async () => {
      const dragId = draggingIdRef.current;
      const target = dropTargetRef.current;

      if (dragId && target) {
        const currentSessions = sessionsRef.current;
        const dragged = currentSessions.find(s => s.id === dragId);
        const targetSession = currentSessions.find(s => s.row === target.row && s.col === target.col);

        if (dragged) {
          const srcRow = dragged.row;
          const srcCol = dragged.col;

          // Optimistic update
          setSessions(prev => prev.map(s => {
            if (s.id === dragId) return { ...s, row: target.row, col: target.col };
            if (targetSession && s.id === targetSession.id) return { ...s, row: srcRow, col: srcCol };
            return s;
          }));

          // Persist to backend
          try {
            await api.moveSession(dragId, target.row, target.col);
            if (targetSession) {
              await api.moveSession(targetSession.id, srcRow, srcCol);
            }
          } catch (err) {
            console.error('Move failed:', err);
            // Revert on failure
            setSessions(prev => prev.map(s => {
              if (s.id === dragId) return { ...s, row: srcRow, col: srcCol };
              if (targetSession && s.id === targetSession.id) return { ...s, row: target.row, col: target.col };
              return s;
            }));
          }
        }
      }

      setDraggingId(null);
      setDropTarget(null);
      draggingIdRef.current = null;
      dropTargetRef.current = null;
    };

    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
    return () => {
      document.removeEventListener('mousemove', onMouseMove);
      document.removeEventListener('mouseup', onMouseUp);
    };
  }, [draggingId, getGridCell]);

  const addPositions = getAddPositions(sessions);

  const allPositions = [
    ...sessions.map(s => ({ row: s.row, col: s.col })),
    ...addPositions,
  ];
  const numCols = allPositions.length > 0 ? Math.max(...allPositions.map(p => p.col)) + 1 : 1;
  const numRows = allPositions.length > 0 ? Math.max(...allPositions.map(p => p.row)) + 1 : 1;

  if (loading) {
    return (
      <div style={styles.outer}>
        <div style={styles.loading}>Loading sessions\u2026</div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flex: 1, overflow: 'hidden', height: '100%' }}>
      <div style={{ ...styles.outer, flex: 1 }}>
      {/* AI toggle button (bottom-right of workspace) */}
      <button
        onClick={() => setAiOpen(o => !o)}
        title={aiOpen ? 'Close AI assistant' : 'Open AI assistant'}
        style={{
          position: 'fixed', bottom: 16, right: aiOpen ? 400 : 16,
          zIndex: 1000, background: '#4a9eff', border: 'none', borderRadius: '50%',
          width: 44, height: 44, fontSize: 20, cursor: 'pointer',
          boxShadow: '0 2px 8px rgba(0,0,0,0.4)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          transition: 'right 0.2s ease',
        }}
      >
        🤖
      </button>
      <div
        ref={gridRef}
        style={{
          ...styles.grid,
          gridTemplateColumns: `repeat(${numCols}, ${tile.w}px)`,
          gridTemplateRows: `repeat(${numRows}, ${tile.h}px)`,
          cursor: draggingId ? 'grabbing' : undefined,
        }}
      >
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
              onRename={handleRename}
              onTitleMouseDown={handleTitleMouseDown}
              isDragging={draggingId === session.id}
              isDropTarget={dropTarget?.row === session.row && dropTarget?.col === session.col}
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

      {/* Drag ghost */}
      {draggingId && (
        <div style={{
          position: 'fixed',
          left: ghostPos.x - tile.w / 2,
          top: ghostPos.y - CHROME_H / 2,
          width: tile.w,
          height: tile.h,
          border: '2px solid #7c6af7',
          borderRadius: 6,
          background: 'rgba(124, 106, 247, 0.12)',
          pointerEvents: 'none',
          zIndex: 1000,
        }} />
      )}

      {dialogPos && (
        <ConnectionDialog
          onConnect={handleAddSession}
          onClose={() => setDialogPos(null)}
          suggestedRow={dialogPos.row}
          suggestedCol={dialogPos.col}
          defaultExecCommand={execCommand}
        />
      )}
      </div>{/* end scrollable grid wrapper */}
      <AISidebar
        getTerminalContext={termContextRef.current}
        isOpen={aiOpen}
        onClose={() => setAiOpen(false)}
      />
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  outer: {
    flex: 1,
    overflowX: 'auto',
    overflowY: 'scroll',
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
