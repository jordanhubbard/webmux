import { useState, useEffect, useCallback, useRef } from 'react';
import { VncTile } from './VncTile';
import { RdpTile } from './RdpTile';
import { VncConnectionDialog } from './VncConnectionDialog';
import { RdpConnectionDialog } from './RdpConnectionDialog';
import { VncFullscreen } from './VncFullscreen';
import { RdpFullscreen } from './RdpFullscreen';
import { api } from '../utils/api';
import type { VncSession, RdpSession, CreateVncSessionRequest, CreateRdpSessionRequest } from '../types';

const GAP = 8;
const TILE_W = 320;
const TILE_H = 268;
const CHROME_H = 30;

type AnySession = VncSession | RdpSession;

type DialogState =
  | { type: 'none' }
  | { type: 'picker'; row: number; col: number }
  | { type: 'vnc'; row: number; col: number }
  | { type: 'rdp'; row: number; col: number };

type FullscreenState =
  | { type: 'none' }
  | { type: 'vnc'; session: VncSession }
  | { type: 'rdp'; session: RdpSession };

function getAddPositions(sessions: AnySession[]): { row: number; col: number }[] {
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
  row: number; col: number; isEmpty: boolean; onClick: () => void;
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

function ProtocolPicker({ row, col, onPick, onClose }: {
  row: number; col: number;
  onPick: (proto: 'vnc' | 'rdp') => void;
  onClose: () => void;
}) {
  return (
    <div
      style={{
        position: 'fixed', inset: 0,
        background: 'rgba(0,0,0,0.7)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 1000,
      }}
      onClick={e => e.target === e.currentTarget && onClose()}
    >
      <div style={{
        background: '#1a1a2e',
        border: '1px solid #333366',
        borderRadius: 8,
        padding: 24,
        width: 340,
        boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
      }}>
        <div style={{ fontSize: 15, fontWeight: 700, color: '#e0e0e0', marginBottom: 20, textAlign: 'center' }}>
          Add Graphics Session
        </div>
        <div style={{ display: 'flex', gap: 12 }}>
          <button
            style={pickerBtnStyle('#333366')}
            onClick={() => onPick('vnc')}
          >
            <span style={{ fontSize: 28, display: 'block', marginBottom: 8 }}>🖥</span>
            <span style={{ fontWeight: 600, fontSize: 14 }}>VNC</span>
            <span style={{ fontSize: 11, color: '#888', marginTop: 4, display: 'block' }}>
              Virtual Network Computing
            </span>
          </button>
          <button
            style={pickerBtnStyle('#1a2a66')}
            onClick={() => onPick('rdp')}
          >
            <span style={{ fontSize: 28, display: 'block', marginBottom: 8 }}>🪟</span>
            <span style={{ fontWeight: 600, fontSize: 14 }}>RDP</span>
            <span style={{ fontSize: 11, color: '#888', marginTop: 4, display: 'block' }}>
              Windows Remote Desktop
            </span>
          </button>
        </div>
        <div style={{ marginTop: 16, fontSize: 11, color: '#555', textAlign: 'center' }}>
          Position: row {row}, col {col}
        </div>
      </div>
    </div>
  );
}

function pickerBtnStyle(border: string): React.CSSProperties {
  return {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    background: '#0d0d1a',
    border: `1px solid ${border}`,
    borderRadius: 8,
    padding: '16px 8px',
    color: '#e0e0e0',
    cursor: 'pointer',
    transition: 'border-color 0.2s, background 0.2s',
  };
}

export function GraphicsWorkspace() {
  const [vncSessions, setVncSessions] = useState<VncSession[]>([]);
  const [rdpSessions, setRdpSessions] = useState<RdpSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [fullscreen, setFullscreen] = useState<FullscreenState>({ type: 'none' });
  const [dialog, setDialog] = useState<DialogState>({ type: 'none' });

  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ row: number; col: number } | null>(null);
  const [ghostPos, setGhostPos] = useState({ x: 0, y: 0 });
  const gridRef = useRef<HTMLDivElement>(null);

  const vncPasswordsRef = useRef(new Map<string, string>());

  const allSessionsRef = useRef<AnySession[]>([]);
  const draggingIdRef = useRef<string | null>(null);
  const dropTargetRef = useRef<{ row: number; col: number } | null>(null);

  const allSessions: AnySession[] = [...vncSessions, ...rdpSessions];
  useEffect(() => { allSessionsRef.current = allSessions; }, [vncSessions, rdpSessions]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { draggingIdRef.current = draggingId; }, [draggingId]);
  useEffect(() => { dropTargetRef.current = dropTarget; }, [dropTarget]);

  useEffect(() => {
    Promise.all([api.getVncSessions(), api.getRdpSessions()])
      .then(([vnc, rdp]) => { setVncSessions(vnc); setRdpSessions(rdp); })
      .catch(err => console.error('Failed to load graphics sessions:', err))
      .finally(() => setLoading(false));
  }, []);

  const handleAddVncSession = useCallback(async (req: CreateVncSessionRequest, password: string) => {
    const session = await api.createVncSession(req);
    if (password) vncPasswordsRef.current.set(session.id, password);
    setVncSessions(prev => [...prev, session]);
    setDialog({ type: 'none' });
  }, []);

  const handleAddRdpSession = useCallback(async (req: CreateRdpSessionRequest, password: string) => {
    const session = await api.createRdpSession({ ...req, rdp_password: password });
    setRdpSessions(prev => [...prev, session]);
    setDialog({ type: 'none' });
  }, []);

  const handleCloseVnc = useCallback(async (id: string) => {
    await api.deleteVncSession(id);
    vncPasswordsRef.current.delete(id);
    setVncSessions(prev => prev.filter(s => s.id !== id));
    setFullscreen(prev => (prev.type === 'vnc' && prev.session.id === id) ? { type: 'none' } : prev);
  }, []);

  const handleCloseRdp = useCallback(async (id: string) => {
    await api.deleteRdpSession(id);
    setRdpSessions(prev => prev.filter(s => s.id !== id));
    setFullscreen(prev => (prev.type === 'rdp' && prev.session.id === id) ? { type: 'none' } : prev);
  }, []);

  const handleReconnectVnc = useCallback(async (id: string) => {
    try {
      const updated = await api.reconnectVncSession(id);
      setVncSessions(prev => prev.map(s => s.id === id ? updated : s));
    } catch (err) { console.error('Reconnect error:', err); }
  }, []);

  const handleReconnectRdp = useCallback(async (id: string) => {
    try {
      const updated = await api.reconnectRdpSession(id);
      setRdpSessions(prev => prev.map(s => s.id === id ? updated : s));
    } catch (err) { console.error('Reconnect error:', err); }
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

  useEffect(() => {
    if (!draggingId) return;

    const onMouseMove = (e: MouseEvent) => {
      setGhostPos({ x: e.clientX, y: e.clientY });
      const cell = getGridCell(e.clientX, e.clientY);
      if (cell) {
        const sessionAtCell = allSessionsRef.current.find(s => s.row === cell.row && s.col === cell.col);
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
        const sessions = allSessionsRef.current;
        const dragged = sessions.find(s => s.id === dragId);
        const targetSession = sessions.find(s => s.row === target.row && s.col === target.col);

        if (dragged && targetSession) {
          const srcRow = dragged.row;
          const srcCol = dragged.col;

          // Optimistic update
          const swap = (prev: AnySession[]): AnySession[] =>
            prev.map(s => {
              if (s.id === dragId) return { ...s, row: target.row, col: target.col } as AnySession;
              if (s.id === targetSession.id) return { ...s, row: srcRow, col: srcCol } as AnySession;
              return s;
            });
          if (dragged.kind === 'vnc') setVncSessions(prev => swap(prev) as VncSession[]);
          else setRdpSessions(prev => swap(prev) as RdpSession[]);
          if (targetSession.kind === 'vnc' && targetSession.id !== dragged.id)
            setVncSessions(prev => swap(prev) as VncSession[]);
          else if (targetSession.kind === 'rdp' && targetSession.id !== dragged.id)
            setRdpSessions(prev => swap(prev) as RdpSession[]);

          // Persist
          try {
            if (dragged.kind === 'vnc') await api.moveVncSession(dragId, target.row, target.col);
            else await api.moveRdpSession(dragId, target.row, target.col);
            if (targetSession.kind === 'vnc') await api.moveVncSession(targetSession.id, srcRow, srcCol);
            else await api.moveRdpSession(targetSession.id, srcRow, srcCol);
          } catch (err) {
            console.error('Move failed:', err);
            // Revert on failure
            const revert = (prev: AnySession[]): AnySession[] =>
              prev.map(s => {
                if (s.id === dragId) return { ...s, row: srcRow, col: srcCol } as AnySession;
                if (s.id === targetSession.id) return { ...s, row: target.row, col: target.col } as AnySession;
                return s;
              });
            if (dragged.kind === 'vnc') setVncSessions(prev => revert(prev) as VncSession[]);
            else setRdpSessions(prev => revert(prev) as RdpSession[]);
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
  if (fullscreen.type === 'vnc') {
    return (
      <VncFullscreen
        session={fullscreen.session}
        vncPassword={vncPasswordsRef.current.get(fullscreen.session.id)}
        onBack={() => setFullscreen({ type: 'none' })}
        onDisconnect={() => handleCloseVnc(fullscreen.session.id)}
      />
    );
  }
  if (fullscreen.type === 'rdp') {
    return (
      <RdpFullscreen
        session={fullscreen.session}
        onBack={() => setFullscreen({ type: 'none' })}
        onDisconnect={() => handleCloseRdp(fullscreen.session.id)}
      />
    );
  }

  const addPositions = getAddPositions(allSessions);
  const allPositions = [
    ...allSessions.map(s => ({ row: s.row, col: s.col })),
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
        {vncSessions.map(session => (
          <div
            key={session.id}
            style={{ gridColumn: session.col + 1, gridRow: session.row + 1, minHeight: 0, minWidth: 0, display: 'flex' }}
          >
            <VncTile
              session={session}
              vncPassword={vncPasswordsRef.current.get(session.id)}
              onDoubleClick={() => setFullscreen({ type: 'vnc', session })}
              onClose={() => handleCloseVnc(session.id)}
              onReconnect={() => handleReconnectVnc(session.id)}
              onTitleMouseDown={e => handleTitleMouseDown(session.id, e)}
              isDragging={draggingId === session.id}
              isDropTarget={dropTarget?.row === session.row && dropTarget?.col === session.col}
            />
          </div>
        ))}

        {rdpSessions.map(session => (
          <div
            key={session.id}
            style={{ gridColumn: session.col + 1, gridRow: session.row + 1, minHeight: 0, minWidth: 0, display: 'flex' }}
          >
            <RdpTile
              session={session}
              onDoubleClick={() => setFullscreen({ type: 'rdp', session })}
              onClose={() => handleCloseRdp(session.id)}
              onReconnect={() => handleReconnectRdp(session.id)}
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
            isEmpty={allSessions.length === 0}
            onClick={() => setDialog({ type: 'picker', row: pos.row, col: pos.col })}
          />
        ))}
      </div>

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

      {dialog.type === 'picker' && (
        <ProtocolPicker
          row={dialog.row}
          col={dialog.col}
          onPick={proto => setDialog({ type: proto, row: dialog.row, col: dialog.col })}
          onClose={() => setDialog({ type: 'none' })}
        />
      )}

      {dialog.type === 'vnc' && (
        <VncConnectionDialog
          onConnect={handleAddVncSession}
          onClose={() => setDialog({ type: 'none' })}
          suggestedRow={dialog.row}
          suggestedCol={dialog.col}
        />
      )}

      {dialog.type === 'rdp' && (
        <RdpConnectionDialog
          onConnect={handleAddRdpSession}
          onClose={() => setDialog({ type: 'none' })}
          suggestedRow={dialog.row}
          suggestedCol={dialog.col}
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
