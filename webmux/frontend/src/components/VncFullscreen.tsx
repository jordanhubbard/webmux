import { useState, useRef, useCallback } from 'react';
import { VncViewer } from './VncViewer';
import type { VncSession } from '../types';

interface VncFullscreenProps {
  session: VncSession;
  vncPassword?: string;
  onBack: () => void;
  onDisconnect: () => void;
}

const TOP_BAR_H = 36;

export function VncFullscreen({ session, vncPassword, onBack, onDisconnect }: VncFullscreenProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const rfbRef = useRef<any>(null);

  const handleSendCtrlAltDel = useCallback(() => {
    rfbRef.current?.sendCtrlAltDel();
    setMenuOpen(false);
  }, []);

  const handlePasteClipboard = useCallback(() => {
    navigator.clipboard.readText().then(text => {
      rfbRef.current?.clipboardPasteFrom(text);
    }).catch(() => {
      // Clipboard permission denied or not available — ignore silently.
    });
    setMenuOpen(false);
  }, []);

  const handleDisconnect = useCallback(() => {
    setMenuOpen(false);
    onDisconnect();
  }, [onDisconnect]);

  const toggleMenu = useCallback(() => {
    setMenuOpen(prev => !prev);
  }, []);

  return (
    <div style={styles.overlay}>
      {/* Top bar */}
      <div style={styles.topBar}>
        <button style={styles.backBtn} onClick={onBack} title="Back to grid">
          &#9664; Back
        </button>

        <span style={styles.sessionTitle} title={session.title}>
          {session.title}
        </span>

        <div style={styles.menuContainer}>
          <button style={styles.optionsBtn} onClick={toggleMenu} title="VNC Options">
            &#8943; Options &#9662;
          </button>

          {menuOpen && (
            <>
              {/* Backdrop to close menu on outside click */}
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

      {/* VNC viewer fills remaining space */}
      <div style={styles.viewerContainer}>
        <VncViewer
          sessionId={session.id}
          vncPassword={vncPassword}
          mode="fullscreen"
          rfbRef={rfbRef}
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
    borderBottom: '1px solid #2a2a5a',
  },
  backBtn: {
    background: 'none',
    border: '1px solid #7c6af7',
    color: '#7c6af7',
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
    border: '1px solid #333366',
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
    border: '1px solid #333366',
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
