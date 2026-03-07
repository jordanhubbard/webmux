import { useState, useCallback } from 'react';
import { Terminal } from './Terminal';
import type { Session, ConnectionState } from '../types';

interface TileProps {
  session: Session;
  fontSize: number;
  onClose: (id: string) => void;
  onSplitRight: (id: string) => void;
  onSplitBelow: (id: string) => void;
  onReconnect: (id: string) => void;
}

export function Tile({ session, fontSize, onClose, onSplitRight, onSplitBelow, onReconnect }: TileProps) {
  const [state, setState] = useState<ConnectionState>(session.state);
  const [viewerCount, setViewerCount] = useState(1);
  const [focusOwner, setFocusOwner] = useState<string | undefined>(undefined);
  const [hasFocus, setHasFocus] = useState(false);

  const handleStateChange = useCallback((newState: ConnectionState) => {
    setState(newState);
  }, []);

  const handleViewerUpdate = useCallback((count: number, owner?: string) => {
    setViewerCount(count);
    setFocusOwner(owner);
  }, []);

  const handleFocusRequest = useCallback(() => {
    setHasFocus(true);
  }, []);

  const stateColor = state === 'connected' ? '#4aaa6a' :
    state === 'connecting' ? '#caaa4a' :
    state === 'error' ? '#ff5555' : '#888888';

  const stateIcon = state === 'connected' ? '●' :
    state === 'connecting' ? '◐' :
    state === 'error' ? '✗' : '○';

  return (
    <div style={styles.tile}>
      {/* Chrome header */}
      <div style={styles.chrome}>
        <div style={styles.chromeLeft}>
          <span style={{ ...styles.stateIndicator, color: stateColor }}>{stateIcon}</span>
          <span style={styles.title} title={session.title}>{session.title}</span>
          <span style={styles.transport}>{session.transport.toUpperCase()}</span>
        </div>
        <div style={styles.chromeRight}>
          {viewerCount > 1 && (
            <span style={styles.viewers} title={`${viewerCount} viewers`}>
              👁 {viewerCount}
            </span>
          )}
          {focusOwner && (
            <span style={styles.focusBadge} title={`Focus: ${focusOwner}`}>🔏</span>
          )}
          <button style={styles.chromeBtn} onClick={() => onSplitRight(session.id)} title="Split right">⊢</button>
          <button style={styles.chromeBtn} onClick={() => onSplitBelow(session.id)} title="Split below">⊤</button>
          {(state === 'disconnected' || state === 'error') && (
            <button style={{ ...styles.chromeBtn, color: '#caaa4a' }} onClick={() => onReconnect(session.id)} title="Reconnect">↺</button>
          )}
          <button style={{ ...styles.chromeBtn, color: '#ff8888' }} onClick={() => onClose(session.id)} title="Close">✕</button>
        </div>
      </div>

      {/* Terminal body */}
      <div style={styles.termContainer}>
        <Terminal
          sessionId={session.id}
          fontSize={fontSize}
          state={state}
          onStateChange={handleStateChange}
          onViewerUpdate={handleViewerUpdate}
          hasFocus={hasFocus}
          onFocusRequest={handleFocusRequest}
        />
      </div>
    </div>
  );
}

const TILE_W = 660;
const TILE_H = 440;

const styles: Record<string, React.CSSProperties> = {
  tile: {
    display: 'flex',
    flexDirection: 'column',
    width: TILE_W,
    height: TILE_H,
    border: '1px solid #333366',
    borderRadius: 6,
    overflow: 'hidden',
    background: '#0d0d1a',
    flexShrink: 0,
  },
  chrome: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    height: 30,
    padding: '0 8px',
    background: '#16163a',
    borderBottom: '1px solid #2a2a5a',
    flexShrink: 0,
  },
  chromeLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    overflow: 'hidden',
  },
  stateIndicator: {
    fontSize: 10,
    flexShrink: 0,
  },
  title: {
    fontSize: 12,
    color: '#ccc',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: 300,
  },
  transport: {
    fontSize: 9,
    color: '#7c6af7',
    background: '#1a1a3a',
    padding: '1px 4px',
    borderRadius: 2,
    flexShrink: 0,
  },
  chromeRight: {
    display: 'flex',
    alignItems: 'center',
    gap: 2,
    flexShrink: 0,
  },
  viewers: {
    fontSize: 10,
    color: '#888',
    marginRight: 4,
  },
  focusBadge: {
    fontSize: 10,
    marginRight: 4,
  },
  chromeBtn: {
    background: 'none',
    border: 'none',
    color: '#666',
    fontSize: 12,
    cursor: 'pointer',
    padding: '2px 4px',
    borderRadius: 2,
    lineHeight: 1,
  },
  termContainer: {
    flex: 1,
    overflow: 'hidden',
  },
};
