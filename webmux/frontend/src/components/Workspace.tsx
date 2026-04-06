import { useState, useEffect, useCallback, useRef } from 'react';
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

interface WorkspaceExtraProps {
  globalAutoScroll: boolean;
  globalAutoScrollVersion: number;
  onGlobalAutoScrollChange: (on: boolean) => void;
  globalLock: boolean;
  globalLockVersion: number;
  onGlobalLockChange: (on: boolean) => void;
}

export function Workspace({ fontSize, termCols, termRows, globalAutoScroll, globalAutoScrollVersion, onGlobalAutoScrollChange, globalLock, globalLockVersion, onGlobalLockChange }: WorkspaceProps & WorkspaceExtraProps) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogPos, setDialogPos] = useState<{ row: number; col: number } | null>(null);
  const [autoScrollOverrides, setAutoScrollOverrides] = useState<Map<string, boolean>>(new Map());
  const [lockOverrides, setLockOverrides] = useState<Map<string, boolean>>(new Map());
  const [collapsedSessions, setCollapsedSessions] = useState<Set<string>>(() => {
    try {
      const saved = localStorage.getItem('webmux_collapsed');
      return saved ? new Set(JSON.parse(saved)) : new Set();
    } catch { return new Set(); }
  });

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

  // Clear all per-window overrides when user explicitly clicks global toggle
  useEffect(() => {
    setAutoScrollOverrides(new Map());
  }, [globalAutoScrollVersion]); // intentionally only depends on version counter

  const handleAutoScrollToggle = useCallback((sessionId: string) => {
    setAutoScrollOverrides(prev => {
      const next = new Map(prev);
      for (const s of sessionsRef.current) {
        if (!next.has(s.id)) {
          next.set(s.id, globalAutoScroll);
        }
      }
      const current = next.get(sessionId)!;
      next.set(sessionId, !current);
      return next;
    });
  }, [globalAutoScroll]);

  // Sync global indicator with per-window overrides
  useEffect(() => {
    if (sessions.length === 0 || autoScrollOverrides.size === 0) return;
    const allOn = sessions.every(s => (autoScrollOverrides.get(s.id) ?? globalAutoScroll) === true);
    const anyOff = sessions.some(s => (autoScrollOverrides.get(s.id) ?? globalAutoScroll) === false);
    if (globalAutoScroll && anyOff) {
      onGlobalAutoScrollChange(false);
    } else if (!globalAutoScroll && allOn) {
      onGlobalAutoScrollChange(true);
    }
  }, [autoScrollOverrides, sessions, globalAutoScroll, onGlobalAutoScrollChange]);

  // Clear lock overrides when user explicitly clicks global lock toggle
  useEffect(() => {
    setLockOverrides(new Map());
  }, [globalLockVersion]); // intentionally only depends on version counter

  const handleLockToggle = useCallback((sessionId: string) => {
    setLockOverrides(prev => {
      const next = new Map(prev);
      for (const s of sessionsRef.current) {
        if (!next.has(s.id)) {
          next.set(s.id, globalLock);
        }
      }
      const current = next.get(sessionId)!;
      next.set(sessionId, !current);
      return next;
    });
  }, [globalLock]);

  // Sync global lock indicator with per-window overrides
  useEffect(() => {
    if (sessions.length === 0 || lockOverrides.size === 0) return;
    const allLocked = sessions.every(s => (lockOverrides.get(s.id) ?? globalLock) === true);
    const anyUnlocked = sessions.some(s => (lockOverrides.get(s.id) ?? globalLock) === false);
    if (globalLock && anyUnlocked) {
      onGlobalLockChange(false);
    } else if (!globalLock && allLocked) {
      onGlobalLockChange(true);
    }
  }, [lockOverrides, sessions, globalLock, onGlobalLockChange]);

  const handleToggleCollapse = useCallback((sessionId: string) => {
    setCollapsedSessions(prev => {
      const next = new Set(prev);
      if (next.has(sessionId)) { next.delete(sessionId); } else { next.add(sessionId); }
      localStorage.setItem('webmux_collapsed', JSON.stringify([...next]));
      return next;
    });
  }, []);

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

  const visibleSessions = sessions.filter(s => !collapsedSessions.has(s.id));

  // Use full session set for column count so grid width stays stable
  const baseCols = sessions.length > 0 ? Math.max(...sessions.map(s => s.col)) + 1 : 1;

  // Compact visible sessions: maintain relative order (row,col) but pack into consecutive cells
  const sorted = [...visibleSessions].sort((a, b) => a.row - b.row || a.col - b.col);
  const compactPositions = new Map<string, { row: number; col: number }>();
  sorted.forEach((s, i) => {
    compactPositions.set(s.id, { row: Math.floor(i / baseCols), col: i % baseCols });
  });

  const addPositions = getAddPositions(sorted.map(s => ({ ...s, ...compactPositions.get(s.id)! })));

  const allPositions = [
    ...Array.from(compactPositions.values()),
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
      <div style={styles.hint}>Hold Shift to scroll</div>
      {sessions.length > 0 && (
        <div style={styles.dock}>
          {[...sessions].sort((a, b) => a.title.localeCompare(b.title)).map(session => {
            const isMinimized = collapsedSessions.has(session.id);
            return (
              <button
                key={session.id}
                style={{
                  ...styles.dockItem,
                  opacity: isMinimized ? 0.5 : 1,
                  borderColor: isMinimized ? '#222244' : '#333366',
                }}
                onClick={() => handleToggleCollapse(session.id)}
                title={isMinimized ? `Show: ${session.title}` : `Minimize: ${session.title}`}
              >
                <span style={{ color: isMinimized ? '#666' : '#4aaa6a', fontSize: 8 }}>{'●'}</span>
                <span style={styles.dockTitle}>{session.title}</span>
              </button>
            );
          })}
        </div>
      )}
      <div
        ref={gridRef}
        style={{
          ...styles.grid,
          gridTemplateColumns: `repeat(${numCols}, ${tile.w}px)`,
          gridTemplateRows: `repeat(${numRows}, ${tile.h}px)`,
          cursor: draggingId ? 'grabbing' : undefined,
        }}
      >
        {visibleSessions.map(session => {
          const pos = compactPositions.get(session.id)!;
          return (<div
            key={session.id}
            style={{
              gridColumn: pos.col + 1,
              gridRow: pos.row + 1,
              minHeight: 0,
              minWidth: 0,
              display: 'flex',
            }}
          >
            <Tile
              session={session}
              fontSize={fontSize}
              autoScroll={autoScrollOverrides.get(session.id) ?? globalAutoScroll}
              onAutoScrollToggle={handleAutoScrollToggle}
              locked={lockOverrides.get(session.id) ?? globalLock}
              onLockToggle={handleLockToggle}
              collapsed={collapsedSessions.has(session.id)}
              onToggleCollapse={handleToggleCollapse}
              onClose={handleClose}
              onReconnect={handleReconnect}
              onRename={handleRename}
              onTitleMouseDown={handleTitleMouseDown}
              isDragging={draggingId === session.id}
              isDropTarget={dropTarget?.row === session.row && dropTarget?.col === session.col}
            />
          </div>);
        })}

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
    boxSizing: 'border-box',
  },
  hint: {
    padding: '3px 10px',
    fontSize: 11,
    color: '#444',
    userSelect: 'none',
    pointerEvents: 'none',
  },
  loading: {
    color: '#888',
    fontSize: 14,
    padding: 32,
    textAlign: 'center',
  },
  dock: {
    display: 'flex',
    flexWrap: 'wrap' as const,
    gap: 4,
    padding: '6px 8px',
    background: '#12122a',
    borderBottom: '1px solid #2a2a5a',
  },
  dockItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    padding: '4px 10px',
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: 4,
    cursor: 'pointer',
    color: '#ccc',
    fontSize: 12,
  },
  dockTitle: {
    maxWidth: 150,
    overflow: 'hidden' as const,
    textOverflow: 'ellipsis' as const,
    whiteSpace: 'nowrap' as const,
  },
};
