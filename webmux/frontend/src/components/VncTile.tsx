import { useState, useCallback } from 'react';
import { VncViewer } from './VncViewer';
import type { VncSession, ConnectionState } from '../types';

interface VncTileProps {
  session: VncSession;
  vncPassword?: string;
  isDropTarget?: boolean;
  isDragging?: boolean;
  onDoubleClick: () => void;
  onClose: () => void;
  onReconnect: () => void;
  onTitleMouseDown?: (e: React.MouseEvent) => void;
}

const CHROME_H = 28;
const BODY_H = 240;

export function VncTile({
  session,
  vncPassword,
  isDropTarget,
  isDragging,
  onDoubleClick,
  onClose,
  onReconnect,
  onTitleMouseDown,
}: VncTileProps) {
  const [localState, setLocalState] = useState<ConnectionState>(session.state);

  const handleStateChange = useCallback((state: ConnectionState) => {
    setLocalState(state);
  }, []);

  const handleChromeMouseDown = useCallback((e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('button')) return;
    onTitleMouseDown?.(e);
  }, [onTitleMouseDown]);

  const stateColor =
    localState === 'connected' ? '#4aaa6a' :
    localState === 'connecting' ? '#caaa4a' :
    localState === 'error' ? '#ff5555' : '#888888';

  const stateIcon =
    localState === 'connected' ? '\u25cf' :
    localState === 'connecting' ? '\u25d0' :
    localState === 'error' ? '\u2717' : '\u25cb';

  const borderColor = isDropTarget ? '#7c6af7' :
    localState === 'connected' ? '#7c6af7' :
    localState === 'connecting' ? '#888888' :
    localState === 'error' ? '#e05050' : '#555555';

  const boxShadow = isDropTarget
    ? '0 0 12px rgba(124, 106, 247, 0.6)'
    : localState === 'connected'
      ? '0 0 8px rgba(124, 106, 247, 0.4)'
      : 'none';

  return (
    <div
      style={{
        gridColumn: session.col + 1,
        gridRow: session.row + 1,
        width: 320,
        height: CHROME_H + BODY_H,
        display: 'flex',
        flexDirection: 'column',
        border: '2px solid',
        borderColor,
        borderRadius: 6,
        overflow: 'hidden',
        background: '#0d0d1a',
        transition: 'border-color 0.15s, box-shadow 0.15s, opacity 0.15s',
        boxSizing: 'border-box',
        boxShadow,
        opacity: isDragging ? 0.35 : 1,
      }}
    >
      {/* Chrome bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          height: CHROME_H,
          padding: '0 8px',
          background: '#16163a',
          borderBottom: '1px solid #2a2a5a',
          flexShrink: 0,
          userSelect: 'none',
          cursor: onTitleMouseDown ? 'grab' : 'default',
        }}
        onMouseDown={handleChromeMouseDown}
      >
        <div style={styles.chromeLeft}>
          <span style={{ ...styles.stateIndicator, color: stateColor }}>{stateIcon}</span>
          <span style={styles.title} title={session.title}>{session.title}</span>
          <span style={styles.transport}>VNC</span>
        </div>
        <div style={styles.chromeRight}>
          {(localState === 'disconnected' || localState === 'error') && (
            <button
              style={{ ...styles.chromeBtn, color: '#caaa4a' }}
              onClick={onReconnect}
              title="Reconnect"
            >
              {'\u21ba'}
            </button>
          )}
          <button
            style={{ ...styles.chromeBtn, color: '#ff8888' }}
            onClick={onClose}
            title="Close"
          >
            {'\u2715'}
          </button>
        </div>
      </div>

      {/* Body */}
      <div
        style={{ flex: 1, overflow: 'hidden', cursor: 'pointer' }}
        onDoubleClick={onDoubleClick}
      >
        <VncViewer
          sessionId={session.id}
          vncPassword={vncPassword}
          mode="thumbnail"
          onStateChange={handleStateChange}
        />
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
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
    maxWidth: 180,
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
};
