interface ReconnectOverlayProps {
  onReconnect: () => void;
}

/** Presents the primary recovery action over a disconnected window. */
export function ReconnectOverlay({ onReconnect }: ReconnectOverlayProps) {
  return (
    <div style={styles.overlay}>
      <button
        type="button"
        style={styles.button}
        onClick={onReconnect}
        aria-label="Reconnect"
      >
        <span aria-hidden="true">{'\u21bb'}</span>
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
    padding: 8,
    color: '#caaa4a',
    background: 'transparent',
    border: 0,
    cursor: 'pointer',
    font: 'inherit',
    fontSize: 64,
    fontWeight: 300,
    lineHeight: 1,
    textShadow: '0 0 20px rgba(202, 170, 74, 0.45)',
  },
};
