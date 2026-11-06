// Scroll strip pinned to the right edge of a terminal tile, for touch devices.
import React from 'react';

export const SCROLL_STRIP_WIDTH = 20;

interface TerminalScrollStripProps {
  // delta is in pages: -1 = one screenful up, +1 = one screenful down.
  onScroll: (delta: number) => void;
}

/**
 * A thin vertical strip pinned to the right edge of a terminal tile, split into
 * two equal half-height tap zones: the whole top half scrolls up, the whole
 * bottom half scrolls down (large, forgiving targets — no precise aiming). A
 * faint divider separates the zones. Used on touch devices where xterm's
 * touch-drag scrollback is unreliable.
 */
export function TerminalScrollStrip({ onScroll }: TerminalScrollStripProps) {
  // Prevent the button from stealing focus/selection from the terminal on tap.
  const hold = (e: React.MouseEvent) => e.preventDefault();
  return (
    <div style={styles.strip}>
      <button
        style={{ ...styles.zone, ...styles.topZone }}
        onMouseDown={hold}
        onClick={() => onScroll(-1)}
        title="Scroll up"
        aria-label="Scroll up"
      >&#9650;</button>
      <button
        style={styles.zone}
        onMouseDown={hold}
        onClick={() => onScroll(1)}
        title="Scroll down"
        aria-label="Scroll down"
      >&#9660;</button>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  strip: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    width: SCROLL_STRIP_WIDTH,
    display: 'flex',
    flexDirection: 'column',
    background: 'rgba(20, 20, 40, 0.55)',
    borderLeft: '1px solid #333366',
    zIndex: 5,
  },
  // Each zone fills half the strip height and is tappable anywhere within it.
  zone: {
    flex: 1,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'none',
    border: 'none',
    color: '#8888cc',
    fontSize: 12,
    lineHeight: 1,
    cursor: 'pointer',
    padding: 0,
    width: '100%',
  },
  topZone: {
    borderBottom: '1px solid #333366',
  },
};
