interface ReconnectOverlayProps {
  onReconnect: () => void;
}

/** Presents the primary recovery action over a disconnected window. */
export function ReconnectOverlay({ onReconnect }: ReconnectOverlayProps) {
  return (
    <div style={styles.overlay}>
      <button type="button" style={styles.button} onClick={onReconnect}>
        <span aria-hidden="true" style={styles.icon}>{'\u21ba'}</span>
        <span>Reconnect</span>
      </button>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'absolute',
    inset: 0,
    zIndex: 2,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'rgba(13, 13, 26, 0.72)',
  },
  button: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
    minWidth: 140,
    padding: '18px 28px',
    color: '#f8f8f2',
    background: '#252555',
    border: '1px solid #7c6af7',
    borderRadius: 8,
    boxShadow: '0 0 20px rgba(124, 106, 247, 0.35)',
    cursor: 'pointer',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 600,
  },
  icon: {
    color: '#caaa4a',
    fontSize: 36,
    lineHeight: 1,
  },
};
