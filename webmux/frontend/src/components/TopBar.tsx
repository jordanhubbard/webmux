import { useState } from 'react';
import type { AuthState } from '../hooks/useAuth';
import { useInputBroadcast } from '../contexts/InputBroadcastContext';
import { HelpDialog } from './HelpDialog';

interface TopBarProps {
  auth: AuthState;
  fontSize: number;
  onFontSizeChange: (size: number) => void;
  termCols: number;
  termRows: number;
  onTermSizeChange: (cols: number, rows: number) => void;
  onNewAccount: () => void;
  secureMode: boolean;
  currentUser: string | null;
  aiPanelOpen: boolean;
  onToggleAiPanel: () => void;
}

export function TopBar({ auth, fontSize, onFontSizeChange, termCols, termRows, onTermSizeChange, onNewAccount, secureMode, currentUser, aiPanelOpen, onToggleAiPanel }: TopBarProps) {
  const { broadcastMode, setBroadcastMode } = useInputBroadcast();
  const [showHelp, setShowHelp] = useState(false);

  return (
    <>
    {showHelp && <HelpDialog onClose={() => setShowHelp(false)} />}
    <div style={styles.bar}>
      <div style={styles.left}>
        <span style={styles.logo}>{'\u25a6'} WebMux</span>
        <button
          style={{
            ...styles.broadcastBtn,
            background: broadcastMode ? '#e8a030' : '#1a1a3a',
            color: broadcastMode ? '#000' : '#aaa',
            borderColor: broadcastMode ? '#e8a030' : '#333366',
          }}
          onMouseDown={e => e.preventDefault()}
          onClick={() => setBroadcastMode(!broadcastMode)}
          title={broadcastMode ? 'Type to All: ON — input goes to every pane' : 'Type to All: OFF — input goes to focused pane only'}
        >
          {broadcastMode ? 'Type to All: ON' : 'Type to All'}
        </button>
      </div>

      <div style={styles.right}>
        <div style={styles.fontControls}>
          <button
            style={styles.iconBtn}
            onClick={() => onFontSizeChange(Math.max(8, fontSize - 1))}
            title="Decrease font size"
          >
            A-
          </button>
          <span style={styles.fontSize}>{fontSize}px</span>
          <button
            style={styles.iconBtn}
            onClick={() => onFontSizeChange(Math.min(32, fontSize + 1))}
            title="Increase font size"
          >
            A+
          </button>
        </div>

        <div style={styles.termSizeControls}>
          <button
            style={styles.iconBtn}
            onClick={() => onTermSizeChange(Math.max(40, termCols - 10), termRows)}
            title="Decrease columns"
          >
            C-
          </button>
          <span style={styles.termSize}>{termCols}×{termRows}</span>
          <button
            style={styles.iconBtn}
            onClick={() => onTermSizeChange(Math.min(240, termCols + 10), termRows)}
            title="Increase columns"
          >
            C+
          </button>
          <button
            style={styles.iconBtn}
            onClick={() => onTermSizeChange(termCols, Math.max(10, termRows - 5))}
            title="Decrease rows"
          >
            R-
          </button>
          <button
            style={styles.iconBtn}
            onClick={() => onTermSizeChange(termCols, Math.min(80, termRows + 5))}
            title="Increase rows"
          >
            R+
          </button>
        </div>

        <div style={{
          ...styles.modeBadge,
          background: secureMode ? '#1a3a2a' : '#3a2a0a',
          borderColor: secureMode ? '#2a6a4a' : '#8a6a0a',
          color: secureMode ? '#4aaa6a' : '#caaa4a',
        }}>
          {secureMode ? 'Secure' : 'Trusted'}
        </div>

        {auth.isAuthenticated && auth.authStatus?.mode !== 'none' && (
          <>
            {currentUser && <span style={styles.userBadge}>{currentUser}</span>}
            <button style={styles.iconBtn} onClick={onNewAccount} title="Create a new account / session collection">
              + Account
            </button>
            <button style={styles.iconBtn} onClick={auth.logout} title="Sign out">
              Sign out
            </button>
          </>
        )}
        <button
          style={{
            ...styles.iconBtn,
            background: aiPanelOpen ? '#2a1a5a' : '#1a1a3a',
            borderColor: aiPanelOpen ? '#7c6af7' : '#333366',
            color: aiPanelOpen ? '#7c6af7' : '#aaa',
          }}
          onClick={onToggleAiPanel}
          title={aiPanelOpen ? 'Hide AI assistant' : 'Show AI assistant'}
        >
          AI
        </button>
        <button style={styles.helpBtn} onClick={() => setShowHelp(true)} title="Usage help">?</button>
      </div>
    </div>
    </>
  );
}

const styles: Record<string, React.CSSProperties> = {
  bar: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    height: 44,
    background: '#12122a',
    borderBottom: '1px solid #333366',
    padding: '0 16px',
    flexShrink: 0,
    zIndex: 100,
  },
  left: {
    display: 'flex',
    alignItems: 'center',
    gap: 16,
  },
  logo: {
    fontSize: 16,
    fontWeight: 700,
    color: '#7c6af7',
    letterSpacing: 1,
  },
  broadcastBtn: {
    border: '1px solid',
    borderRadius: 4,
    padding: '5px 12px',
    fontSize: 12,
    fontWeight: 600,
    cursor: 'pointer',
    transition: 'background 0.15s, color 0.15s, border-color 0.15s',
  },
  right: {
    display: 'flex',
    alignItems: 'center',
    gap: 12,
  },
  fontControls: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
  },
  iconBtn: {
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: 4,
    padding: '4px 8px',
    color: '#aaa',
    fontSize: 12,
    cursor: 'pointer',
  },
  fontSize: {
    color: '#aaa',
    fontSize: 12,
    minWidth: 36,
    textAlign: 'center',
  },
  termSizeControls: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
  },
  termSize: {
    color: '#aaa',
    fontSize: 12,
    minWidth: 48,
    textAlign: 'center',
    fontFamily: 'monospace',
  },
  modeBadge: {
    border: '1px solid',
    borderRadius: 4,
    padding: '3px 8px',
    fontSize: 11,
    fontWeight: 600,
    letterSpacing: 0.3,
  },
  userBadge: {
    color: '#7c6af7',
    fontSize: 12,
    fontWeight: 600,
  },
  helpBtn: {
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: '50%',
    width: 22,
    height: 22,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#7c6af7',
    fontSize: 13,
    fontWeight: 700,
    cursor: 'pointer',
    flexShrink: 0,
    padding: 0,
    lineHeight: 1,
  },
};
