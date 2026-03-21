import { useState, useCallback, useRef } from 'react';
import { Terminal, type TerminalHandle } from './Terminal';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';
import { api } from '../utils/api';
import type { Session, ConnectionState } from '../types';

interface TileProps {
  session: Session;
  fontSize: number;
  onClose: (id: string) => void;
  onReconnect: (id: string) => void;
  onRename: (id: string, title: string) => void;
  onTitleMouseDown?: (sessionId: string, e: React.MouseEvent) => void;
  isDragging?: boolean;
  isDropTarget?: boolean;
}

export function Tile({ session, fontSize, onClose, onReconnect, onRename, onTitleMouseDown, isDragging, isDropTarget }: TileProps) {
  const [state, setState] = useState<ConnectionState>(session.state);
  const [viewerCount, setViewerCount] = useState(1);
  const [editing, setEditing] = useState(false);
  const [editValue, setEditValue] = useState(session.title);
  const { focusedSessionId, broadcastMode, broadcastExcluded, toggleBroadcastExclude } = useInputBroadcast();
  const isExcluded = broadcastExcluded.has(session.id);
  const termHandleRef = useRef<TerminalHandle>(null);
  const inputRef = useRef<HTMLInputElement>(null);

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

  const handleChromeMouseDown = useCallback((e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('button')) return;
    if ((e.target as HTMLElement).closest('input')) return;
    onTitleMouseDown?.(session.id, e);
  }, [onTitleMouseDown, session.id]);

  const commitRename = useCallback(() => {
    const trimmed = editValue.trim();
    if (trimmed && trimmed !== session.title) {
      onRename(session.id, trimmed);
    } else {
      setEditValue(session.title);
    }
    setEditing(false);
  }, [editValue, session.id, session.title, onRename]);

  const handleTitleDoubleClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setEditValue(session.title);
    setEditing(true);
    setTimeout(() => inputRef.current?.select(), 0);
  }, [session.title]);

  const [fileDragOver, setFileDragOver] = useState(false);

  const handleFileDrop = useCallback(async (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setFileDragOver(false);
    const files = e.dataTransfer.files;
    if (files.length === 0) return;
    try {
      const result = await api.uploadFile(files[0]);
      termHandleRef.current?.sendInput(result.path + ' ');
    } catch (err) {
      console.error('File upload failed:', err);
    }
  }, []);

  const stateColor = state === 'connected' ? '#4aaa6a' :
    state === 'connecting' ? '#caaa4a' :
    state === 'error' ? '#ff5555' : '#888888';

  const stateIcon = state === 'connected' ? '\u25cf' :
    state === 'connecting' ? '\u25d0' :
    state === 'error' ? '\u2717' : '\u25cb';

  const borderColor = isDropTarget
    ? '#7c6af7'
    : broadcastMode
      ? (isExcluded ? '#333366' : '#e8a030')
      : isFocused
        ? '#7c6af7'
        : '#333366';

  return (
    <div
      onWheel={e => { if (!e.shiftKey) e.stopPropagation(); }}
      onDragOver={e => { e.preventDefault(); e.stopPropagation(); setFileDragOver(true); }}
      onDragLeave={e => { e.preventDefault(); e.stopPropagation(); setFileDragOver(false); }}
      onDrop={handleFileDrop}
      style={{
        ...styles.tile,
        borderColor: fileDragOver ? '#50fa7b' : borderColor,
        boxShadow: fileDragOver
          ? '0 0 12px rgba(80, 250, 123, 0.6)'
          : isDropTarget
          ? '0 0 12px rgba(124, 106, 247, 0.6)'
          : broadcastMode
            ? (isExcluded ? 'none' : '0 0 8px rgba(232, 160, 48, 0.4)')
            : isFocused
              ? '0 0 8px rgba(124, 106, 247, 0.4)'
              : 'none',
        opacity: isDragging ? 0.35 : 1,
      }}>
      <div
        style={{
          ...styles.chrome,
          cursor: onTitleMouseDown ? 'grab' : 'default',
        }}
        onMouseDown={handleChromeMouseDown}
      >
        <div style={styles.chromeLeft}>
          <span style={{ ...styles.stateIndicator, color: stateColor }}>{stateIcon}</span>
          {editing ? (
            <input
              ref={inputRef}
              value={editValue}
              onChange={e => setEditValue(e.target.value)}
              onBlur={commitRename}
              onKeyDown={e => {
                if (e.key === 'Enter') commitRename();
                if (e.key === 'Escape') { setEditValue(session.title); setEditing(false); }
                e.stopPropagation();
              }}
              style={styles.titleInput}
              maxLength={128}
            />
          ) : (
            <span
              style={styles.title}
              title={`${session.title} (double-click to rename)`}
              onDoubleClick={handleTitleDoubleClick}
            >{session.title}</span>
          )}
          <span style={styles.transport}>{session.transport.toUpperCase()}</span>
        </div>
        <div style={styles.chromeRight}>
          {broadcastMode && (
            <button
              style={{
                ...styles.chromeBtn,
                color: isExcluded ? '#666' : '#e8a030',
                fontSize: 10,
              }}
              onClick={() => toggleBroadcastExclude(session.id)}
              title={isExcluded ? 'Excluded from broadcast (click to include)' : 'Included in broadcast (click to exclude)'}
            >{isExcluded ? '\u25cb' : '\u25cf'}</button>
          )}
          {viewerCount > 1 && (
            <span style={styles.viewers} title={`${viewerCount} viewers`}>
              {viewerCount}
            </span>
          )}
          <button
            style={{ ...styles.chromeBtn, color: '#8888cc' }}
            onClick={() => termHandleRef.current?.scrollToBottom()}
            title="Scroll to bottom"
          >&#8595;</button>
          {(state === 'disconnected' || state === 'error') && (
            <button style={{ ...styles.chromeBtn, color: '#caaa4a' }} onClick={() => onReconnect(session.id)} title="Reconnect">{'\u21ba'}</button>
          )}
          <button style={{ ...styles.chromeBtn, color: '#ff8888' }} onClick={() => onClose(session.id)} title="Close">{'\u2715'}</button>
        </div>
      </div>

      <div style={styles.termContainer}>
        <Terminal
          ref={termHandleRef}
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
    transition: 'border-color 0.15s, box-shadow 0.15s, opacity 0.15s',
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
    userSelect: 'none',
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
    cursor: 'text',
  },
  titleInput: {
    fontSize: 12,
    color: '#fff',
    background: '#0d0d1a',
    border: '1px solid #7c6af7',
    borderRadius: 2,
    padding: '0 4px',
    outline: 'none',
    maxWidth: 300,
    fontFamily: 'inherit',
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
