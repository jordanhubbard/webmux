import { useState, useCallback } from 'react';
import { Terminal } from './Terminal';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';
import type { Session, ConnectionState } from '../types';

interface TileProps {
  session: Session;
  fontSize: number;
  onClose: (id: string) => void;
  onReconnect: (id: string) => void;
}

export function Tile({ session, fontSize, onClose, onReconnect }: TileProps) {
  const [state, setState] = useState<ConnectionState>(session.state);
  const [viewerCount, setViewerCount] = useState(1);
  const { focusedSessionId, broadcastMode } = useInputBroadcast();

  const isFocused = focusedSessionId === session.id;

  const handleStateChange = useCallback((newState: ConnectionState) => {
    setState(newState);
  }, []);

  const handleViewerUpdate = useCallback((count: number, _owner?: string) => {
    setViewerCount(count);
  }, []);

  const handleFocusGained = useCallback(() => {
    // Terminal handles setting focusedSessionId via context
  }, []);

  const stateColor = state === 'connected' ? '#4aaa6a' :
    state === 'connecting' ? '#caaa4a' :
    state === 'error' ? '#ff5555' : '#888888';

  const stateIcon = state === 'connected' ? '\u25cf' :
    state === 'connecting' ? '\u25d0' :
    state === 'error' ? '\u2717' : '\u25cb';

  const borderColor = broadcastMode
    ? '#e8a030'
    : isFocused
      ? '#7c6af7'
      : '#333366';

  return (
    <div style={{
      ...styles.tile,
      borderColor,
      boxShadow: broadcastMode
        ? '0 0 8px rgba(232, 160, 48, 0.4)'
        : isFocused
          ? '0 0 8px rgba(124, 106, 247, 0.4)'
          : 'none',
    }}>
      <div style={styles.chrome}>
        <div style={styles.chromeLeft}>
          <span style={{ ...styles.stateIndicator, color: stateColor }}>{stateIcon}</span>
          <span style={styles.title} title={session.title}>{session.title}</span>
          <span style={styles.transport}>{session.transport.toUpperCase()}</span>
        </div>
        <div style={styles.chromeRight}>
          {viewerCount > 1 && (
            <span style={styles.viewers} title={`${viewerCount} viewers`}>
              {viewerCount}
            </span>
          )}
          {(state === 'disconnected' || state === 'error') && (
            <button style={{ ...styles.chromeBtn, color: '#caaa4a' }} onClick={() => onReconnect(session.id)} title="Reconnect">{'\u21ba'}</button>
          )}
          <button style={{ ...styles.chromeBtn, color: '#ff8888' }} onClick={() => onClose(session.id)} title="Close">{'\u2715'}</button>
        </div>
      </div>

      <div style={styles.termContainer}>
        <Terminal
          sessionId={session.id}
          fontSize={fontSize}
          state={state}
          onStateChange={handleStateChange}
          onViewerUpdate={handleViewerUpdate}
          onFocusGained={handleFocusGained}
        />
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  tile: {
    display: 'flex',
    flexDirection: 'column',
    width: '100%',
    height: '100%',
    border: '2px solid #333366',
    borderRadius: 6,
    overflow: 'hidden',
    background: '#0d0d1a',
    transition: 'border-color 0.15s, box-shadow 0.15s',
    boxSizing: 'border-box',
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
