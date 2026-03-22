import { useState, useCallback } from 'react';

interface ClaudeAuthOverlayProps {
  authUrl: string;
  onSendInput: (data: string) => void;
}

export function ClaudeAuthOverlay({ authUrl, onSendInput }: ClaudeAuthOverlayProps) {
  const [credential, setCredential] = useState('');
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(authUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable — fallback via selection
    }
  }, [authUrl]);

  const handleOpenUrl = useCallback(() => {
    window.open(authUrl, '_blank', 'noopener,noreferrer');
  }, [authUrl]);

  const handleSubmitCredential = useCallback(() => {
    const trimmed = credential.trim();
    if (!trimmed) return;
    onSendInput(trimmed + '\n');
    setCredential('');
  }, [credential, onSendInput]);

  return (
    <div style={styles.overlay}>
      <div style={styles.box}>
        <div style={styles.header}>
          <span style={styles.robot}>🤖</span>
          <span style={styles.title}>Claude Authentication Required</span>
        </div>

        <p style={styles.description}>
          Open the URL below to authenticate with Claude, then paste the authorization code here.
        </p>

        <div style={styles.urlBox}>
          <span style={styles.urlText}>{authUrl}</span>
        </div>

        <div style={styles.urlActions}>
          <button style={styles.openBtn} onClick={handleOpenUrl}>
            Open in Browser
          </button>
          <button style={{ ...styles.copyBtn, ...(copied ? styles.copiedBtn : {}) }} onClick={handleCopy}>
            {copied ? 'Copied!' : 'Copy URL'}
          </button>
        </div>

        <div style={styles.credentialSection}>
          <label style={styles.label}>Paste authorization code:</label>
          <div style={styles.credentialRow}>
            <input
              style={styles.credentialInput}
              type="text"
              placeholder="Paste code here..."
              value={credential}
              onChange={e => setCredential(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleSubmitCredential(); }}
            />
            <button
              style={styles.submitBtn}
              onClick={handleSubmitCredential}
              disabled={!credential.trim()}
            >
              Submit
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'absolute',
    inset: 0,
    background: 'rgba(0, 0, 0, 0.82)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 50,
  },
  box: {
    background: '#1a1a2e',
    border: '1px solid #7c6af7',
    borderRadius: 8,
    padding: 20,
    maxWidth: 420,
    width: '90%',
    boxShadow: '0 0 24px rgba(124, 106, 247, 0.4)',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  },
  robot: {
    fontSize: 20,
  },
  title: {
    fontSize: 14,
    fontWeight: 700,
    color: '#e0e0e0',
  },
  description: {
    fontSize: 12,
    color: '#aaa',
    margin: 0,
    lineHeight: 1.5,
  },
  urlBox: {
    background: '#0d0d1a',
    border: '1px solid #333366',
    borderRadius: 4,
    padding: '8px 10px',
    wordBreak: 'break-all',
  },
  urlText: {
    fontSize: 11,
    color: '#8be9fd',
    fontFamily: 'monospace',
  },
  urlActions: {
    display: 'flex',
    gap: 8,
  },
  openBtn: {
    flex: 1,
    background: '#7c6af7',
    border: 'none',
    borderRadius: 4,
    padding: '7px 12px',
    color: '#fff',
    fontSize: 12,
    fontWeight: 600,
    cursor: 'pointer',
  },
  copyBtn: {
    flex: 1,
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: 4,
    padding: '7px 12px',
    color: '#aaa',
    fontSize: 12,
    cursor: 'pointer',
    transition: 'background 0.15s, color 0.15s',
  },
  copiedBtn: {
    background: '#1a3a2a',
    borderColor: '#2a6a4a',
    color: '#4aaa6a',
  },
  credentialSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  label: {
    fontSize: 11,
    color: '#aaa',
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  credentialRow: {
    display: 'flex',
    gap: 6,
  },
  credentialInput: {
    flex: 1,
    background: '#0d0d1a',
    border: '1px solid #333',
    borderRadius: 4,
    padding: '7px 10px',
    color: '#e0e0e0',
    fontSize: 12,
    outline: 'none',
    fontFamily: 'monospace',
  },
  submitBtn: {
    background: '#2a4a3a',
    border: '1px solid #3a6a4a',
    borderRadius: 4,
    padding: '7px 14px',
    color: '#80cc90',
    fontSize: 12,
    fontWeight: 500,
    cursor: 'pointer',
  },
};
