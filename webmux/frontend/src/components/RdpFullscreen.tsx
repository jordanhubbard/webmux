import { useState, useRef, useCallback } from 'react';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import Guacamole from 'guacamole-common-js';
import { RdpViewer } from './RdpViewer';
import type { RdpSession } from '../types';

interface RdpFullscreenProps {
  session: RdpSession;
  onBack: () => void;
  onDisconnect: () => void;
}

const TOP_BAR_H = 36;

// X11 keysyms for Ctrl+Alt+Del
const KEY_CTRL = 0xFFE3;
const KEY_ALT = 0xFFE9;
const KEY_DELETE = 0xFFFF;

export function RdpFullscreen({ session, onBack, onDisconnect }: RdpFullscreenProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const clientRef = useRef<any>(null);

  const handleSendCtrlAltDel = useCallback(() => {
    const c = clientRef.current;
    if (c) {
      c.sendKeyEvent(1, KEY_CTRL);
      c.sendKeyEvent(1, KEY_ALT);
      c.sendKeyEvent(1, KEY_DELETE);
      c.sendKeyEvent(0, KEY_DELETE);
      c.sendKeyEvent(0, KEY_ALT);
      c.sendKeyEvent(0, KEY_CTRL);
    }
    setMenuOpen(false);
  }, []);

  const handlePasteClipboard = useCallback(() => {
    navigator.clipboard.readText().then(text => {
      const c = clientRef.current;
      if (!c) return;
      try {
        const stream = c.createClipboardStream('text/plain');
        const writer = new Guacamole.StringWriter(stream);
        writer.sendText(text);
        writer.sendEnd();
      } catch {
        // Clipboard stream not available — ignore silently
      }
    }).catch(() => {});
    setMenuOpen(false);
  }, []);

  const handleDisconnect = useCallback(() => {
    setMenuOpen(false);
    onDisconnect();
  }, [onDisconnect]);

  return (
    <div style={styles.overlay}>
      <div style={styles.topBar}>
        <button style={styles.backBtn} onClick={onBack} title="Back to grid">
          &#9664; Back
        </button>

        <span style={styles.sessionTitle} title={session.title}>
          {session.title}
        </span>

        <div style={styles.menuContainer}>
          <button style={styles.optionsBtn} onClick={() => setMenuOpen(p => !p)} title="RDP Options">
            &#8943; Options &#9662;
          </button>

          {menuOpen && (
            <>
              <div style={styles.menuBackdrop} onClick={() => setMenuOpen(false)} />
              <div style={styles.menu}>
                <div style={styles.menuItem} onClick={handleSendCtrlAltDel}>
                  Send Ctrl+Alt+Del
                </div>
                <div style={styles.menuItem} onClick={handlePasteClipboard}>
                  Paste Clipboard
                </div>
                <div style={styles.menuSeparator}>─────────────</div>
                <div style={{ ...styles.menuItem, color: '#ff8888' }} onClick={handleDisconnect}>
                  Disconnect
                </div>
              </div>
            </>
          )}
        </div>
      </div>

      <div style={styles.viewerContainer}>
        <RdpViewer
          sessionId={session.id}
          mode="fullscreen"
          clientRef={clientRef}
        />
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    inset: 0,
    zIndex: 2000,
    background: '#000',
    display: 'flex',
    flexDirection: 'column',
  },
  topBar: {
    position: 'relative',
    zIndex: 2001,
    height: TOP_BAR_H,
    background: 'rgba(13,13,26,0.85)',
    display: 'flex',
    alignItems: 'center',
    padding: '0 12px',
    gap: 12,
    flexShrink: 0,
    borderBottom: '1px solid #1a2a66',
  },
  backBtn: {
    background: 'none',
    border: '1px solid #4a7af7',
    color: '#4a7af7',
    fontSize: 13,
    cursor: 'pointer',
    padding: '3px 10px',
    borderRadius: 4,
    lineHeight: 1,
    flexShrink: 0,
  },
  sessionTitle: {
    flex: 1,
    textAlign: 'center',
    fontSize: 13,
    color: '#ccc',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  menuContainer: {
    position: 'relative',
    flexShrink: 0,
  },
  optionsBtn: {
    background: 'none',
    border: '1px solid #1a2a66',
    color: '#aaa',
    fontSize: 13,
    cursor: 'pointer',
    padding: '3px 10px',
    borderRadius: 4,
    lineHeight: 1,
  },
  menuBackdrop: {
    position: 'fixed',
    inset: 0,
    zIndex: 2001,
  },
  menu: {
    position: 'absolute',
    top: '100%',
    right: 0,
    background: '#1a1a3a',
    border: '1px solid #1a2a66',
    borderRadius: 4,
    minWidth: 180,
    zIndex: 2002,
    overflow: 'hidden',
    marginTop: 4,
  },
  menuItem: {
    padding: '8px 14px',
    fontSize: 13,
    color: '#ccc',
    cursor: 'pointer',
    userSelect: 'none',
  },
  menuSeparator: {
    padding: '4px 14px',
    fontSize: 11,
    color: '#444',
    userSelect: 'none',
    letterSpacing: 1,
  },
  viewerContainer: {
    flex: 1,
    overflow: 'hidden',
  },
};
