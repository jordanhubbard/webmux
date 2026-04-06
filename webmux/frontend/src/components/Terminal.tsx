import { useEffect, useRef, useState, useCallback, useImperativeHandle, forwardRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { SearchAddon } from '@xterm/addon-search';
import '@xterm/xterm/css/xterm.css';
import type { WebSocketMessage, ConnectionState } from '../types';
import { useWebSocket } from '../hooks/useWebSocket';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';

export interface TerminalHandle {
  scrollToBottom: () => void;
  isAtBottom: () => boolean;
  sendInput: (data: string) => void;
}

interface TerminalProps {
  sessionId: string;
  fontSize: number;
  state: ConnectionState;
  autoScroll: boolean;
  onStateChange: (state: ConnectionState) => void;
  onViewerUpdate: (count: number, focusOwner?: string) => void;
  onFocusGained: () => void;
  onBell?: () => void;
}

export const Terminal = forwardRef<TerminalHandle, TerminalProps>(function Terminal({
  sessionId,
  fontSize,
  state,
  autoScroll,
  onStateChange,
  onViewerUpdate,
  onFocusGained,
  onBell,
}: TerminalProps, ref) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const wsHandleRef = useRef<ReturnType<typeof useWebSocket> | null>(null);
  const userScrolledRef = useRef(false);
  const autoScrollRef = useRef(autoScroll);
  const [showSearch, setShowSearch] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchIndex, setSearchIndex] = useState(-1);
  const [searchCount, setSearchCount] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const { registerSend, unregisterSend, routeInput, setFocusedSessionId, broadcastMode, focusedSessionId } = useInputBroadcast();

  useImperativeHandle(ref, () => ({
    scrollToBottom: () => {
      userScrolledRef.current = false;
      termRef.current?.scrollToBottom();
    },
    isAtBottom: () => !userScrolledRef.current,
    sendInput: (data: string) => {
      wsHandleRef.current?.send({ type: 'input', data });
    },
  }));

  useEffect(() => { autoScrollRef.current = autoScroll; }, [autoScroll]);

  // Keep latest callbacks in refs so xterm/WS handlers never capture stale closures.
  const onStateChangeRef = useRef(onStateChange);
  const onViewerUpdateRef = useRef(onViewerUpdate);
  const onFocusGainedRef = useRef(onFocusGained);
  const onBellRef = useRef(onBell);
  useEffect(() => { onStateChangeRef.current = onStateChange; }, [onStateChange]);
  useEffect(() => { onViewerUpdateRef.current = onViewerUpdate; }, [onViewerUpdate]);
  useEffect(() => { onFocusGainedRef.current = onFocusGained; }, [onFocusGained]);
  useEffect(() => { onBellRef.current = onBell; }, [onBell]);

  // routeInput is a stable callback (useCallback with [] deps) that reads
  // broadcastMode from a ref internally — use it directly without a wrapper ref.
  const routeInputRef = useRef(routeInput);

  const handleMessage = useCallback((msg: WebSocketMessage) => {
    switch (msg.type) {
      case 'output':
        if (msg.data && termRef.current) {
          const shouldScroll = autoScrollRef.current && !userScrolledRef.current;
          if (shouldScroll) {
            termRef.current.write(msg.data, () => {
              termRef.current?.scrollToBottom();
            });
          } else {
            const savedViewport = termRef.current.buffer.active.viewportY;
            termRef.current.write(msg.data, () => {
              termRef.current?.scrollToLine(savedViewport);
            });
          }
        }
        break;
      case 'status':
        if (msg.state) onStateChangeRef.current(msg.state);
        break;
      case 'viewer_join':
      case 'viewer_leave':
        onViewerUpdateRef.current(msg.viewer_count ?? 0, msg.focus_owner);
        break;
      case 'focus':
        onViewerUpdateRef.current(msg.viewer_count ?? 0, msg.focus_owner);
        break;
      default:
        break;
    }
  }, []);

  const wsHandle = useWebSocket({
    sessionId,
    onMessage: handleMessage,
    onOpen: () => onStateChangeRef.current('connected'),
    onClose: () => onStateChangeRef.current('disconnected'),
  });

  useEffect(() => {
    wsHandleRef.current = wsHandle;
  }, [wsHandle]);

  // Register this terminal's send function with the broadcast context
  useEffect(() => {
    const sendInput = (data: string) => {
      wsHandleRef.current?.send({ type: 'input', data });
    };
    registerSend(sessionId, sendInput);
    return () => unregisterSend(sessionId);
  }, [sessionId, registerSend, unregisterSend]);

  // Initialize xterm
  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      theme: {
        background: '#0d0d1a',
        foreground: '#e0e0e0',
        cursor: '#7c6af7',
        selectionBackground: 'rgba(124, 106, 247, 0.3)',
        black: '#1a1a2e',
        brightBlack: '#333333',
        red: '#ff5555',
        brightRed: '#ff8080',
        green: '#50fa7b',
        brightGreen: '#80ffaa',
        yellow: '#f1fa8c',
        brightYellow: '#ffff80',
        blue: '#6272a4',
        brightBlue: '#8080ff',
        magenta: '#ff79c6',
        brightMagenta: '#ffaadd',
        cyan: '#8be9fd',
        brightCyan: '#aaeeff',
        white: '#f8f8f2',
        brightWhite: '#ffffff',
      },
      fontFamily: 'Consolas, Menlo, "DejaVu Sans Mono", monospace',
      fontSize,
      cursorBlink: true,
      macOptionIsMeta: /Mac|iPhone|iPad/.test(navigator.platform),
      allowTransparency: false,
      scrollback: 5000,
      linkHandler: {
        activate: (_event: MouseEvent, uri: string) => {
          window.open(uri, '_blank', 'noopener,noreferrer');
        },
      },
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    const searchAddon = new SearchAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);
    term.loadAddon(searchAddon);
    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;
    searchAddon.onDidChangeResults(({ resultIndex, resultCount }) => {
      setSearchIndex(resultIndex);
      setSearchCount(resultCount);
    });

    // Route input through the broadcast context instead of sending directly
    const dataListener = term.onData((data: string) => {
      routeInputRef.current(sessionId, data);
    });

    const resizeListener = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      wsHandleRef.current?.send({ type: 'resize', cols, rows });
    });

    const bellListener = term.onBell(() => {
      onBellRef.current?.();
    });

    const termEl = containerRef.current!;
    const wheelHandler = (e: WheelEvent) => {
      if (e.deltaY < 0) {
        userScrolledRef.current = true;
      } else if (e.deltaY > 0) {
        requestAnimationFrame(() => {
          if (term.buffer.active.viewportY >= term.buffer.active.baseY) {
            userScrolledRef.current = false;
          }
        });
      }
    };
    termEl.addEventListener('wheel', wheelHandler, { passive: true });

    // Cmd/Ctrl+F to open search
    term.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'f' && e.type === 'keydown') {
        e.preventDefault();
        setShowSearch(true);
        setTimeout(() => searchInputRef.current?.focus(), 0);
        return false;
      }
      return true;
    });

    const el = containerRef.current;
    // Click to focus this terminal
    const clickHandler = () => {
      setFocusedSessionId(sessionId);
      onFocusGainedRef.current();
      term.focus();
    };
    el.addEventListener('mousedown', clickHandler);

    return () => {
      dataListener.dispose();
      resizeListener.dispose();
      bellListener.dispose();
      termEl.removeEventListener('wheel', wheelHandler);
      el.removeEventListener('mousedown', clickHandler);
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
    // Re-run only when the session changes; all live callbacks are accessed via refs.
  }, [sessionId, setFocusedSessionId]);

  // Update font size
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.fontSize = fontSize;
      fitAddonRef.current?.fit();
    }
  }, [fontSize]);

  // When broadcast mode is enabled, re-focus the active terminal so keyboard
  // input flows immediately without requiring the user to click again.
  // (Clicking the TopBar button can cause xterm to lose focus before
  // React's event delegation fires preventDefault.)
  useEffect(() => {
    if (broadcastMode && focusedSessionId === sessionId && termRef.current) {
      termRef.current.focus();
    }
  }, [broadcastMode, focusedSessionId, sessionId]);

  // When autoScroll is re-enabled, snap to bottom
  useEffect(() => {
    if (autoScroll && termRef.current) {
      userScrolledRef.current = false;
      termRef.current.scrollToBottom();
    }
  }, [autoScroll]);

  // Refit on container resize (skip if container is hidden/off-screen)
  useEffect(() => {
    const observer = new ResizeObserver(() => {
      if (containerRef.current && containerRef.current.offsetWidth > 0 && containerRef.current.offsetHeight > 0) {
        fitAddonRef.current?.fit();
      }
    });
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }
    return () => observer.disconnect();
  }, []);

  const overlayColor = state === 'disconnected' ? 'rgba(13,13,26,0.85)' :
    state === 'error' ? 'rgba(60,13,13,0.85)' :
    state === 'connecting' ? 'rgba(13,13,26,0.6)' : 'transparent';
  const showOverlay = state !== 'connected';

  return (
    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
      <div
        ref={containerRef}
        style={{
          width: '100%',
          height: '100%',
          opacity: state === 'connected' ? 1 : 0.4,
        }}
      />
      {showSearch && (
        <div style={{ position: 'absolute', top: 4, right: 4, display: 'flex', gap: 4, zIndex: 10 }}>
          <input
            ref={searchInputRef}
            value={searchQuery}
            onChange={e => {
              setSearchQuery(e.target.value);
              const opts = { caseSensitive: false, decorations: { matchOverviewRuler: '#7c6af7', activeMatchColorOverviewRuler: '#50fa7b', matchBackground: '#7c6af733', activeMatchBackground: '#50fa7b55' } };
              if (e.target.value) { searchAddonRef.current?.findNext(e.target.value, opts); } else { searchAddonRef.current?.clearDecorations(); }
            }}
            onKeyDown={e => {
              const opts = { caseSensitive: false, decorations: { matchOverviewRuler: '#7c6af7', activeMatchColorOverviewRuler: '#50fa7b', matchBackground: '#7c6af733', activeMatchBackground: '#50fa7b55' } };
              if (e.key === 'Enter') { if (e.shiftKey) { searchAddonRef.current?.findPrevious(searchQuery, opts); } else { searchAddonRef.current?.findNext(searchQuery, opts); } }
              if (e.key === 'Escape') { searchAddonRef.current?.clearDecorations(); setShowSearch(false); setSearchQuery(''); setSearchIndex(-1); setSearchCount(0); termRef.current?.focus(); }
              e.stopPropagation();
            }}
            placeholder="Search..."
            style={{ background: '#0d0d1a', border: '1px solid #7c6af7', borderRadius: 3, color: '#e0e0e0', fontSize: 12, padding: '3px 8px', outline: 'none', width: 180 }}
          />
          {searchQuery && (
            <span style={{ color: searchCount > 0 ? '#888' : '#ff5555', fontSize: 11, alignSelf: 'center', whiteSpace: 'nowrap' }}>
              {searchCount > 0 ? `${searchIndex + 1}/${searchCount}` : 'No results'}
            </span>
          )}
          <button onClick={() => { const opts = { caseSensitive: false, decorations: { matchOverviewRuler: '#7c6af7', activeMatchColorOverviewRuler: '#50fa7b', matchBackground: '#7c6af733', activeMatchBackground: '#50fa7b55' } }; searchAddonRef.current?.findPrevious(searchQuery, opts); }} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#aaa', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Previous (Shift+Enter)">{'\u25b2'}</button>
          <button onClick={() => { const opts = { caseSensitive: false, decorations: { matchOverviewRuler: '#7c6af7', activeMatchColorOverviewRuler: '#50fa7b', matchBackground: '#7c6af733', activeMatchBackground: '#50fa7b55' } }; searchAddonRef.current?.findNext(searchQuery, opts); }} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#aaa', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Next (Enter)">{'\u25bc'}</button>
          <button onClick={() => { searchAddonRef.current?.clearDecorations(); setShowSearch(false); setSearchQuery(''); setSearchIndex(-1); setSearchCount(0); termRef.current?.focus(); }} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#ff8888', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Close (Escape)">{'\u2715'}</button>
        </div>
      )}
      {showOverlay && (
        <div style={{
          position: 'absolute',
          inset: 0,
          background: overlayColor,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#aaa',
          fontSize: 13,
          pointerEvents: 'none',
        }}>
          {state === 'connecting' && 'Connecting...'}
          {state === 'disconnected' && 'Disconnected'}
          {state === 'error' && 'Connection error'}
        </div>
      )}
    </div>
  );
});
