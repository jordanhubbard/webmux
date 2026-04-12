import { useState, useEffect, useCallback, useRef } from 'react';
import { VncTile } from './VncTile';
import { VncConnectionDialog } from './VncConnectionDialog';
import { VncFullscreen } from './VncFullscreen';
import { api } from '../utils/api';
import type { VncSession, CreateVncSessionRequest } from '../types';

const GAP = 8;
const TILE_W = 320;
const TILE_H = 268;
const CHROME_H = 30;

function getAddPositions(sessions: VncSession[]): { row: number; col: number }[] {
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

export function VncWorkspace() {
  const [sessions, setSessions] = useState<VncSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [fullscreenSessionId, setFullscreenSessionId] = useState<string | null>(null);
  const [dialogPos, setDialogPos] = useState<{ row: number; col: number } | null>(null);

  // Drag state
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ row: number; col: number } | null>(null);
  const [ghostPos, setGhostPos] = useState({ x: 0, y: 0 });
  const gridRef = useRef<HTMLDivElement>(null);

  // Password storage by session id
  const vncPasswordsRef = useRef(new Map<string, string>());

  // Refs for use in event handlers (avoid stale closures)
  const sessionsRef = useRef<VncSession[]>([]);
  const draggingIdRef = useRef<string | null>(null);
  const dropTargetRef = useRef<{ row: number; col: number } | null>(null);
  useEffect(() => { sessionsRef.current = sessions; }, [sessions]);
  useEffect(() => { draggingIdRef.current = draggingId; }, [draggingId]);
  useEffect(() => {
    dropTargetRef.current = dropTarget;
  }, [dropTarget]);

  useEffect(() => {
    api.getVncSessions()
      .then(setSessions)
      .catch(err => console.error('Failed to load VNC sessions:', err))
      .finally(() => setLoading(false));
  }, []);

  const handleAddVncSession = useCallback(async (req: CreateVncSessionRequest, password: string) => {
    const session = await api.createVncSession(req);
    if (password) vncPasswordsRef.current.set(session.id, password);
    setSessions(prev => [...prev, session]);
    setDialogPos(null);
  }, []);

  const handleCloseSession = useCallback(async (id: string) => {
    await api.deleteVncSession(id);
    vncPasswordsRef.current.delete(id);
    setSessions(prev => prev.filter(s => s.id !== id));
    if (fullscreenSessionId === id) setFullscreenSessionId(null);
  }, [fullscreenSessionId]);

  const handleReconnect = useCallback(async (id: string) => {
    try {
      const updated = await api.reconnectVncSession(id);
      setSessions(prev => prev.map(s => s.id === id ? updated : s));
    } catch (err) {
      console.error('Reconnect error:', err);
    }
  }, []);

  const getGridCell = useCallback((clientX: number, clientY: number): { row: number; col: number } | null => {
    if (!gridRef.current) return null;
    const rect = gridRef.current.getBoundingClientRect();
    const x = clientX - rect.left - GAP;
    const y = clientY - rect.top - GAP;
    if (x < 0 || y < 0) return null;
    const col = Math.floor(x / (TILE_W + GAP));
    const row = Math.floor(y / (TILE_H + GAP));
    if (col < 0 || row < 0) return null;
    return { row, col };
  }, []);

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
        const newTarget = (sessionAtCell && sessionAtCell.id !== draggingIdRef.current) ? cell : null;
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

        if (dragged && targetSession) {
          const srcRow = dragged.row;
          const srcCol = dragged.col;

          // Optimistic update
          setSessions(prev => prev.map(s => {
            if (s.id === dragId) return { ...s, row: target.row, col: target.col };
            if (s.id === targetSession.id) return { ...s, row: srcRow, col: srcCol };
            return s;
          }));

          // Persist to backend
          try {
            await api.moveVncSession(dragId, target.row, target.col);
            await api.moveVncSession(targetSession.id, srcRow, srcCol);
          } catch (err) {
            console.error('Move failed:', err);
            // Revert on failure
            setSessions(prev => prev.map(s => {
              if (s.id === dragId) return { ...s, row: srcRow, col: srcCol };
              if (s.id === targetSession.id) return { ...s, row: target.row, col: target.col };
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

  // Fullscreen rendering
  if (fullscreenSessionId !== null) {
    const fsSession = sessions.find(s => s.id === fullscreenSessionId);
    if (fsSession) {
      return (
        <VncFullscreen
          session={fsSession}
          vncPassword={vncPasswordsRef.current.get(fsSession.id)}
          onBack={() => setFullscreenSessionId(null)}
          onDisconnect={() => handleCloseSession(fsSession.id)}
        />
      );
    }
  }

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
    <div style={styles.outer}>
      <div
        ref={gridRef}
        style={{
          ...styles.grid,
          gridTemplateColumns: `repeat(${numCols}, ${TILE_W}px)`,
          gridTemplateRows: `repeat(${numRows}, ${TILE_H}px)`,
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
            <VncTile
              session={session}
              vncPassword={vncPasswordsRef.current.get(session.id)}
              onDoubleClick={() => setFullscreenSessionId(session.id)}
              onClose={() => handleCloseSession(session.id)}
              onReconnect={() => handleReconnect(session.id)}
              onTitleMouseDown={e => handleTitleMouseDown(session.id, e)}
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
          left: ghostPos.x - TILE_W / 2,
          top: ghostPos.y - CHROME_H / 2,
          width: TILE_W,
          height: TILE_H,
          border: '2px solid #7c6af7',
          borderRadius: 6,
          background: 'rgba(124, 106, 247, 0.12)',
          pointerEvents: 'none',
          zIndex: 1000,
        }} />
      )}

      {dialogPos && (
        <VncConnectionDialog
          onConnect={handleAddVncSession}
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
