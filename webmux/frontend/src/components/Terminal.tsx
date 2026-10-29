import { copyDesktopText, clipboardFiles, installTerminalPaste, MAX_TERMINAL_FILE_BYTES, quoteTerminalPath, terminalKeyHandler } from '../utils/terminalClipboard';
import { api } from '../utils/api';
import { ClipboardDialog } from './ClipboardDialog';
import { useEffect, useRef, useState, useCallback, useImperativeHandle, forwardRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { SearchAddon } from '@xterm/addon-search';
import '@xterm/xterm/css/xterm.css';
import type { WebSocketMessage, ConnectionState, TerminalTheme } from '../types';
import { useWebSocket } from '../hooks/useWebSocket';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';
import { installTerminalQuerySuppressors, isTerminalLocalInput, shouldSuppressTerminalInput } from '../utils/terminalInput';
import { TERMINAL_FONTS_LOADED_EVENT, loadTerminalFontFamily, normalizeTerminalFontFamily } from '../utils/terminalFont';
import { scrollTerminalPages } from '../utils/terminalScroll';

export const DEFAULT_TERMINAL_THEME: TerminalTheme = {
  background: '#0d0d1a',
  foreground: '#e0e0e0',
  cursor: '#7c6af7',
  selectionBackground: 'rgba(124, 106, 247, 0.3)' as unknown as string,
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
};

const SEARCH_OPTIONS = { caseSensitive: false, decorations: { matchOverviewRuler: '#7c6af7', activeMatchColorOverviewRuler: '#50fa7b', matchBackground: '#7c6af733', activeMatchBackground: '#50fa7b55' } };

export interface TerminalHandle {
  acknowledgeBrowser: (id: string) => void;
  scrollToBottom: () => void;
  scrollPages: (delta: number) => void;
  isAtBottom: () => boolean;
  sendInput: (data: string) => void;
  pasteInput: (data: string) => void;
  uploadFiles: (files: File[]) => void;
  focus: () => void;
  openSearch: () => void;
  openClipboard: () => void;
  copySelection: () => void;
  pasteFromDesktop: () => void;
  toggleTranscript: () => void;
}

interface TerminalProps {
  sessionId: string;
  fontSize: number;
  fontFamily?: string;
  state: ConnectionState;
  autoScroll: boolean;
  onStateChange: (state: ConnectionState) => void;
  onViewerUpdate: (count: number, focusOwner?: string) => void;
  onFocusGained: () => void;
  theme?: TerminalTheme | null;
  onBell?: () => void;
  onTranscriptChange?: (enabled: boolean) => void;
  onOpenLink?: (url: string, requestId?: string) => void;
}

export const Terminal = forwardRef<TerminalHandle, TerminalProps>(function Terminal({
  sessionId,
  fontSize,
  fontFamily,
  state,
  autoScroll,
  onStateChange,
  onViewerUpdate,
  onFocusGained,
  theme,
  onBell,
  onTranscriptChange,
  onOpenLink,
}: TerminalProps, ref) {
  const stateRef = useRef(state); stateRef.current = state;
  const localPasteRef = useRef(false);
  const [remoteClipboard, setRemoteClipboard] = useState<string | null>(null);
  const onOpenLinkRef = useRef(onOpenLink);
  onOpenLinkRef.current = onOpenLink;
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const wsHandleRef = useRef<ReturnType<typeof useWebSocket> | null>(null);
  const userScrolledRef = useRef(false);
  const autoScrollRef = useRef(autoScroll);
  const [clipboardText, setClipboardText] = useState<string | null>(null);
  const [inputError, setInputError] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchIndex, setSearchIndex] = useState(-1);
  const [searchCount, setSearchCount] = useState(0);
  const [transcriptEnabled, setTranscriptEnabled] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const { registerSend, unregisterSend, routeInput, setFocusedSessionId, broadcastMode, focusedSessionId } = useInputBroadcast();

  const [uploadNotice, setUploadNotice] = useState('');
  const [uploadedPaths, setUploadedPaths] = useState<string[]>([]);
  const uploadingRef = useRef(false);
  const uploadAbortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; uploadAbortRef.current?.abort(); };
  }, [sessionId]);
  const pasteLocalInput = (data: string) => {
    if (stateRef.current !== 'connected' || !termRef.current) throw new Error('File delivered, but the terminal disconnected. Reconnect and paste the displayed path.');
    localPasteRef.current = true;
    try { termRef.current.paste(data); } finally { localPasteRef.current = false; }
  };
  const transferFiles = async (files: File[]) => {
    if (!files.length) return;
    if (uploadingRef.current) { setInputError('A file transfer is already running. Wait for it to finish, then retry.'); return; }
    if (stateRef.current !== 'connected') { setInputError('Reconnect the terminal before transferring files.'); return; }
    if (files.length > 16) { setInputError('Drop or paste at most 16 files at a time.'); return; }
    if (files.some(file => file.size > MAX_TERMINAL_FILE_BYTES)) { setInputError('Each file must be at most 10 MiB.'); return; }
    uploadingRef.current = true;
    const controller = new AbortController(); uploadAbortRef.current = controller;
    setInputError(''); setUploadedPaths([]);
    try {
      for (const [index, file] of files.entries()) {
        if (!mountedRef.current) break;
        setUploadNotice(`Transferring ${index + 1}/${files.length}: ${file.name || 'clipboard image'}…`);
        const result = await api.uploadTerminalFile(sessionId, file, controller.signal);
        if (!mountedRef.current) break;
        setUploadedPaths(previous => [...previous, result.path]);
        pasteLocalInput(quoteTerminalPath(result.path) + ' ');
        termRef.current?.focus();
        setUploadNotice(`Delivered ${index + 1}/${files.length} to this terminal’s host.`);
      }
    } catch (err) {
      if (mountedRef.current) { setUploadNotice(''); setInputError(err instanceof Error ? err.message : String(err)); }
    } finally { uploadingRef.current = false; }
  };
  const transferRef = useRef(transferFiles); transferRef.current = transferFiles;

  const pasteFromDesktop = async () => {
    if (stateRef.current !== 'connected') { setInputError('Reconnect the terminal before pasting.'); return; }
    try {
      if (navigator.clipboard?.read) {
        const items = await navigator.clipboard.read();
        const files: File[] = [];
        let text = '';
        for (const item of items) {
          const imageType = item.types.find(type => type.startsWith('image/'));
          if (imageType) {
            const blob = await item.getType(imageType);
            files.push(new File([blob], `clipboard-${Date.now()}.${imageType === 'image/jpeg' ? 'jpg' : imageType.split('/')[1]}`, { type: imageType }));
          } else if (item.types.includes('text/plain')) text += await (await item.getType('text/plain')).text();
        }
        if (files.length) await transferRef.current(files);
        else {
          if (new TextEncoder().encode(text).length > 128 * 1024) throw new Error('Paste at most 128 KiB of text at a time.');
          termRef.current?.paste(text);
        }
      } else if (navigator.clipboard?.readText) {
        const text = await navigator.clipboard.readText();
        if (new TextEncoder().encode(text).length > 128 * 1024) throw new Error('Paste at most 128 KiB of text at a time.');
        termRef.current?.paste(text);
      } else throw new Error('Clipboard API unavailable');
    } catch {
      setInputError('Browser clipboard access is unavailable or denied. Focus this terminal and use Cmd+V (Mac) or Ctrl+V / Ctrl+Shift+V to paste text or images directly.');
    }
    termRef.current?.focus();
  };
  const pasteDesktopRef = useRef(pasteFromDesktop); pasteDesktopRef.current = pasteFromDesktop;

  useImperativeHandle(ref, () => ({
    acknowledgeBrowser: (id: string) => { wsHandleRef.current?.send({ type: 'browser_ack', data: id }); },
    scrollToBottom: () => {
      userScrolledRef.current = false;
      termRef.current?.scrollToBottom();
    },
    scrollPages: (delta: number) => {
      if (termRef.current) {
        scrollTerminalPages(termRef.current, delta, userScrolledRef,
          (data) => wsHandleRef.current?.send({ type: 'input', data }));
      }
    },
    isAtBottom: () => !userScrolledRef.current,
    sendInput: (data: string) => {
      wsHandleRef.current?.send({ type: 'input', data });
    },
    pasteInput: pasteLocalInput,
    uploadFiles: files => { void transferRef.current(files); },
    focus: () => {
      termRef.current?.focus();
    },
    openClipboard: () => { setClipboardText(termRef.current?.getSelection() ?? ''); },
    copySelection: () => {
      const text = termRef.current?.getSelection() ?? '';
      if (!text) { setInputError('Select terminal text first. In mouse-enabled apps, hold Option on Mac or Shift elsewhere while selecting.'); return; }
      void copyDesktopText(text).then(copied => { if (!copied) setClipboardText(text); termRef.current?.focus(); });
    },
    pasteFromDesktop: () => { void pasteDesktopRef.current(); },
    openSearch: () => {
      setFocusedSessionId(sessionId);
      onFocusGainedRef.current();
      setShowSearch(true);
      searchInputRef.current?.focus();
    },
    toggleTranscript: () => {
      wsHandleRef.current?.send({ type: 'transcript_toggle' });
      termRef.current?.focus();
    },
  }));

  useEffect(() => {
    if (showSearch) searchInputRef.current?.focus();
  }, [showSearch]);

  useEffect(() => {
    onTranscriptChange?.(transcriptEnabled);
  }, [onTranscriptChange, transcriptEnabled]);

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
      case 'clipboard': {
        const text = msg.data ?? '';
        setRemoteClipboard(text);
        if (document.hasFocus() && document.activeElement === termRef.current?.textarea && navigator.clipboard?.writeText) {
          void navigator.clipboard.writeText(text).then(() => setRemoteClipboard(current => current === text ? null : current)).catch(() => { /* A user click can grant clipboard access. */ });
        }
        break;
      }
      case 'browser_open':
        if (msg.url && onOpenLinkRef.current) {
          onOpenLinkRef.current(msg.url, msg.request_id);
        }
        break;
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
        // Queue the reset with output so old asynchronous writes cannot re-enable modes.
        // viewer_id marks an initial snapshot; connected marks a new PTY run.
        if (msg.viewer_id || msg.state === 'connected') termRef.current?.write('\x18\x1bc');
        if (msg.state) onStateChangeRef.current(msg.state);
        if (typeof msg.transcript_enabled === 'boolean') setTranscriptEnabled(msg.transcript_enabled);
        break;
      case 'transcript_status':
        if (typeof msg.transcript_enabled === 'boolean') setTranscriptEnabled(msg.transcript_enabled);
        if (msg.message) console.error(`Transcript logging: ${msg.message}`);
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
    onSendError: setInputError,
    onOpen: () => {
      // The first fit can precede the resize listener. Always send the current
      // viewport when the socket opens, including after reconnecting a viewer.
      const term = termRef.current;
      if (term) wsHandleRef.current?.send({ type: 'resize', cols: term.cols, rows: term.rows });
      onStateChangeRef.current('connected');
    },
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

    const normalizedFontFamily = normalizeTerminalFontFamily(fontFamily);
    const openLink = (event: MouseEvent, uri: string) => {
      event.preventDefault();
      if (onOpenLinkRef.current) onOpenLinkRef.current(uri);
      else window.open(uri, '_blank', 'noopener,noreferrer');
    };
    const term = new XTerm({
      theme: { ...DEFAULT_TERMINAL_THEME, ...(theme || {}) },
      fontFamily: normalizedFontFamily,
      fontSize,
      cursorBlink: true,
      macOptionClickForcesSelection: true,
      macOptionIsMeta: /Mac|iPhone|iPad/.test(navigator.platform),
      allowTransparency: false,
      // Search highlighting uses xterm's proposed decoration API.
      allowProposedApi: true,
      scrollback: 5000,
      linkHandler: { activate: openLink },
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon(openLink);
    const searchAddon = new SearchAddon();
    const terminalQuerySuppressor = installTerminalQuerySuppressors(term);
    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);
    term.loadAddon(searchAddon);
    term.open(containerRef.current);
    fitAddon.fit();

    term.attachCustomKeyEventHandler(event => {
      if (/Mac|iPhone|iPad/.test(navigator.platform) && event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === 'v') {
        if (event.type === 'keydown') { event.preventDefault(); void pasteDesktopRef.current(); }
        return false;
      }
      return terminalKeyHandler(event, term.hasSelection());
    });
    const removePaste = installTerminalPaste(containerRef.current, term, files => {
      void transferRef.current(files);
    }, () => stateRef.current === 'connected', setInputError);
    termRef.current = term;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;
    void loadTerminalFontFamily(normalizedFontFamily, fontSize).then(() => {
      if (termRef.current !== term) return;
      fitAddon.fit();
      term.refresh(0, Math.max(0, term.rows - 1));
    });
    searchAddon.onDidChangeResults(({ resultIndex, resultCount }) => {
      setSearchIndex(resultIndex);
      setSearchCount(resultCount);
    });

    // Route input through the broadcast context instead of sending directly
    const dataListener = term.onData((data: string) => {
      if (shouldSuppressTerminalInput(data)) return;
      if (localPasteRef.current || isTerminalLocalInput(data)) wsHandleRef.current?.send({ type: 'input', data });
      else routeInputRef.current(sessionId, data);
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

    const el = containerRef.current;
    const focusHandler = () => {
      wsHandleRef.current?.send({ type: 'focus' });
      setFocusedSessionId(sessionId);
      onFocusGainedRef.current();
    };
    // Click to focus this terminal
    const clickHandler = () => {
      focusHandler();
      term.focus();
    };
    el.addEventListener('mousedown', clickHandler);
    el.addEventListener('focusin', focusHandler);

    return () => {
      removePaste();
      dataListener.dispose();
      resizeListener.dispose();
      bellListener.dispose();
      terminalQuerySuppressor.dispose();
      termEl.removeEventListener('wheel', wheelHandler);
      el.removeEventListener('mousedown', clickHandler);
      el.removeEventListener('focusin', focusHandler);
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
    // Re-run only when the session changes; all live callbacks are accessed via refs.
  }, [sessionId, setFocusedSessionId]);

  // Update font settings
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;

    const normalizedFontFamily = normalizeTerminalFontFamily(fontFamily);
    let cancelled = false;
    term.options.fontSize = fontSize;
    term.options.fontFamily = normalizedFontFamily;
    fitAddonRef.current?.fit();

    void loadTerminalFontFamily(normalizedFontFamily, fontSize).then(() => {
      if (cancelled || termRef.current !== term) return;
      fitAddonRef.current?.fit();
      term.refresh(0, Math.max(0, term.rows - 1));
    });

    return () => { cancelled = true; };
  }, [fontFamily, fontSize]);

  useEffect(() => {
    const refreshFontMetrics = () => {
      const term = termRef.current;
      if (!term) return;
      void loadTerminalFontFamily(fontFamily, fontSize).then(() => {
        if (termRef.current !== term) return;
        fitAddonRef.current?.fit();
        term.refresh(0, Math.max(0, term.rows - 1));
      });
    };
    window.addEventListener(TERMINAL_FONTS_LOADED_EVENT, refreshFontMetrics);
    return () => window.removeEventListener(TERMINAL_FONTS_LOADED_EVENT, refreshFontMetrics);
  }, [fontFamily, fontSize]);

  // Apply theme changes without tearing down the terminal (preserves scrollback).
  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme = { ...DEFAULT_TERMINAL_THEME, ...(theme || {}) };
    }
  }, [theme]);

  // When broadcast mode is enabled, re-focus the active terminal so keyboard
  // input flows immediately without requiring the user to click again.
  // (Clicking the TopBar button can cause xterm to lose focus before
  // React's event delegation fires preventDefault.)
  useEffect(() => {
    if (broadcastMode && !showSearch && focusedSessionId === sessionId && termRef.current) {
      termRef.current.focus();
    }
  }, [broadcastMode, focusedSessionId, sessionId, showSearch]);

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
        // Force re-render of buffer after restoring from minimized
        termRef.current?.refresh(0, termRef.current.rows - 1);
      }
    });
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }
    return () => observer.disconnect();
  }, []);

  const findMatch = (direction: 'next' | 'previous') => {
    if (!searchQuery) return;
    if (direction === 'next') searchAddonRef.current?.findNext(searchQuery, SEARCH_OPTIONS);
    else searchAddonRef.current?.findPrevious(searchQuery, SEARCH_OPTIONS);
  };

  const closeSearch = () => {
    searchAddonRef.current?.clearDecorations();
    setShowSearch(false);
    setSearchQuery('');
    setSearchIndex(-1);
    setSearchCount(0);
    termRef.current?.focus();
  };

  const overlayColor = state === 'disconnected' ? 'rgba(13,13,26,0.85)' :
    state === 'error' ? 'rgba(60,13,13,0.85)' :
    state === 'connecting' ? 'rgba(13,13,26,0.6)' : 'transparent';
  const showOverlay = state !== 'connected';

  return (
    <div style={{ position: 'relative', width: '100%', height: '100%' }}
      onDragOver={event => { if (Array.from(event.dataTransfer.types).includes('Files')) { event.preventDefault(); event.stopPropagation(); event.dataTransfer.dropEffect = 'copy'; } }}
      onDrop={event => { if (Array.from(event.dataTransfer.types).includes('Files')) { event.preventDefault(); event.stopPropagation(); void transferRef.current(clipboardFiles(event.dataTransfer)); } }}>
      {(uploadNotice || inputError || uploadedPaths.length > 0) && <div style={{ position: 'absolute', top: 0, left: 0, zIndex: 20, padding: '6px 10px', background: '#242442', color: '#eee', fontSize: 12, maxWidth: '100%', overflowWrap: 'anywhere' }}>
        <span role={inputError ? 'alert' : 'status'}>{inputError || uploadNotice}</span>{' '}
        <button onClick={() => { setUploadNotice(''); setInputError(''); setUploadedPaths([]); }}>Dismiss</button>
        {uploadedPaths.length > 0 && <details><summary>Delivered file paths</summary><div style={{ whiteSpace: 'pre-wrap', userSelect: 'all' }}>{uploadedPaths.join('\n')}</div></details>}
      </div>}
      {remoteClipboard !== null && <div role="status" style={{ position: 'absolute', bottom: 4, left: 4, zIndex: 20, padding: 8, background: '#242442', color: '#eee', maxWidth: 'calc(100% - 8px)' }}>
        Terminal requested a desktop clipboard copy.{' '}
        <button onClick={() => {
          const text = remoteClipboard;
          void copyDesktopText(text).then(copied => {
            if (copied) setRemoteClipboard(current => current === text ? null : current);
            else setClipboardText(text);
          });
        }}>Copy to desktop</button>{' '}
        <button onClick={() => setRemoteClipboard(null)}>Dismiss</button>
      </div>}
      {clipboardText !== null && <ClipboardDialog initialText={clipboardText} onClose={() => { setClipboardText(null); setTimeout(() => termRef.current?.focus(), 0); }} onSend={text => {
        if (state !== 'connected') throw new Error('Terminal is disconnected. Reconnect before sending.');
        termRef.current?.paste(text);
      }} />}
      <div
        ref={containerRef}
        data-1p-ignore
        style={{
          width: '100%',
          height: '100%',
          opacity: state === 'connected' ? 1 : 0.4,
        }}
      />
      {showSearch && (
        <div role="search" aria-label="Terminal scrollback" style={{ position: 'absolute', top: 4, right: 4, maxWidth: 'calc(100% - 8px)', display: 'flex', gap: 4, zIndex: 10 }}>
          <input
            ref={searchInputRef}
            aria-label="Search terminal scrollback"
            value={searchQuery}
            onChange={e => {
              setSearchQuery(e.target.value);
              if (e.target.value) { searchAddonRef.current?.findNext(e.target.value, SEARCH_OPTIONS); } else { searchAddonRef.current?.clearDecorations(); }
            }}
            onKeyDown={e => {
              if (e.key === 'Enter') { e.preventDefault(); findMatch(e.shiftKey ? 'previous' : 'next'); }
              if (e.key === 'Escape') { e.preventDefault(); closeSearch(); }
              e.stopPropagation();
            }}
            placeholder="Search..."
            style={{ background: '#0d0d1a', border: '1px solid #7c6af7', borderRadius: 3, color: '#e0e0e0', fontSize: 12, padding: '3px 8px', outline: 'none', width: 180, minWidth: 0 }}
          />
          {searchQuery && (
            <span style={{ color: searchCount > 0 ? '#888' : '#ff5555', fontSize: 11, alignSelf: 'center', whiteSpace: 'nowrap' }}>
              {searchCount > 0 ? `${searchIndex + 1}/${searchCount}` : 'No results'}
            </span>
          )}
          <button aria-label="Previous match" disabled={!searchQuery} onClick={() => findMatch('previous')} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#aaa', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Previous (Shift+Enter)">{'\u25b2'}</button>
          <button aria-label="Next match" disabled={!searchQuery} onClick={() => findMatch('next')} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#aaa', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Next (Enter)">{'\u25bc'}</button>
          <button aria-label="Close terminal search" onClick={closeSearch} style={{ background: '#1a1a3a', border: '1px solid #333', borderRadius: 3, color: '#ff8888', fontSize: 11, cursor: 'pointer', padding: '2px 6px' }} title="Close (Escape)">{'\u2715'}</button>
        </div>
      )}
      {transcriptEnabled && (
        <div
          role="status"
          aria-label="Session transcript logging active"
          title="Session transcript logging active (use Log in the toolbar to pause)"
          style={{ position: 'absolute', right: 8, bottom: 6, zIndex: 9, color: '#ff6666', background: 'rgba(13,13,26,0.8)', borderRadius: 3, padding: '2px 5px', fontSize: 10, fontWeight: 700, pointerEvents: 'none' }}
        >REC</div>
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
