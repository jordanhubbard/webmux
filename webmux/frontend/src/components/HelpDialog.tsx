import { useEffect } from 'react';

interface HelpDialogProps {
  onClose: () => void;
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section style={styles.section}>
      <h3 style={styles.sectionTitle}>{title}</h3>
      {children}
    </section>
  );
}

function Row({ label, desc }: { label: string; desc: string }) {
  return (
    <div style={styles.row}>
      <span style={styles.label}>{label}</span>
      <span style={styles.desc}>{desc}</span>
    </div>
  );
}

export function HelpDialog({ onClose }: HelpDialogProps) {
  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.dialog} onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="WebMux help">
        <div style={styles.header}>
          <span style={styles.title}>&#9638; WebMux — Usage</span>
          <button style={styles.closeBtn} onClick={onClose} title="Dismiss (Esc)">&#10005;</button>
        </div>

        <div style={styles.body}>
          <Section title="Workspace">
            <Row label="Click +" desc="Open a new terminal session. The + placeholder appears to the right of and below each existing tile." />
            <Row label="Click ✕ (tile)" desc="Close and remove that session." />
            <Row label="Click ↺ (tile)" desc="Reconnect a disconnected or errored session." />
            <Row label="Click terminal" desc="Focus that tile — keyboard input is directed here." />
          </Section>

          <Section title="Scrolling">
            <Row label="Wheel over terminal" desc="Scrolls that terminal's scrollback buffer (5,000 lines)." />
            <Row label="Shift + Wheel" desc="Scrolls the workspace view (pans the tile grid) even when the pointer is over a terminal." />
            <Row label="Drag scrollbar" desc="The workspace scrollbar on the right always scrolls the workspace view." />
          </Section>

          <Section title="Keyboard Input">
            <Row label="Normal typing" desc="Input goes to the currently focused terminal tile." />
            <Row label="Type to All" desc="When enabled (orange bar), every keystroke is sent to all open sessions simultaneously. Click the button again to disable." />
          </Section>

          <Section title="Top Bar Controls">
            <Row label="Type to All" desc="Toggle broadcast mode — sends input to every session at once." />
            <Row label="A− / A+" desc="Decrease or increase the terminal font size (8–32 px, persisted)." />
            <Row label="C− / C+" desc="Decrease or increase terminal columns in steps of 10 (40–240)." />
            <Row label="R− / R+" desc="Decrease or increase terminal rows in steps of 5 (10–80)." />
            <Row label="Secure / Trusted" desc="Shows the current security mode. Secure = HTTPS + JWT auth. Trusted = open access for isolated networks." />
            <Row label="+ Account" desc="Create an additional user account." />
            <Row label="Sign out" desc="Log out of the current session." />
          </Section>

          <Section title="Sessions">
            <Row label="Persistent" desc="Sessions survive browser closes and server restarts — they are auto-reconnected on the next start." />
            <Row label="Multi-viewer" desc="Multiple browser tabs can observe the same session. The tab that last clicked the terminal has keyboard focus." />
            <Row label="SSH / Mosh" desc="Choose transport per session. Mosh is more resilient to intermittent connectivity." />
          </Section>
        </div>

        <div style={styles.footer}>
          <button style={styles.dismissBtn} onClick={onClose}>Dismiss</button>
        </div>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.6)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
  },
  dialog: {
    background: '#12122a',
    border: '1px solid #333366',
    borderRadius: 8,
    width: 620,
    maxWidth: '95vw',
    maxHeight: '85vh',
    display: 'flex',
    flexDirection: 'column',
    boxShadow: '0 8px 40px rgba(0,0,0,0.6)',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #2a2a5a',
    flexShrink: 0,
  },
  title: {
    fontSize: 15,
    fontWeight: 700,
    color: '#7c6af7',
    letterSpacing: 0.5,
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#666',
    fontSize: 14,
    cursor: 'pointer',
    padding: '2px 6px',
    borderRadius: 3,
  },
  body: {
    overflowY: 'auto',
    padding: '8px 16px 16px',
    flex: 1,
  },
  section: {
    marginTop: 16,
  },
  sectionTitle: {
    margin: '0 0 8px',
    fontSize: 11,
    fontWeight: 700,
    color: '#7c6af7',
    textTransform: 'uppercase',
    letterSpacing: 1,
    borderBottom: '1px solid #1e1e3a',
    paddingBottom: 4,
  },
  row: {
    display: 'flex',
    gap: 12,
    marginBottom: 6,
    alignItems: 'baseline',
  },
  label: {
    flexShrink: 0,
    width: 160,
    fontSize: 12,
    fontWeight: 600,
    color: '#ccc',
    fontFamily: 'Consolas, Menlo, monospace',
  },
  desc: {
    fontSize: 12,
    color: '#999',
    lineHeight: 1.5,
  },
  footer: {
    padding: '10px 16px',
    borderTop: '1px solid #2a2a5a',
    display: 'flex',
    justifyContent: 'flex-end',
    flexShrink: 0,
  },
  dismissBtn: {
    background: '#1a1a3a',
    border: '1px solid #333366',
    borderRadius: 4,
    padding: '6px 20px',
    color: '#aaa',
    fontSize: 13,
    cursor: 'pointer',
  },
};
