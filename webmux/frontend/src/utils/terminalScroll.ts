// Helpers for scrolling a terminal by pages, used by the touch scroll strip.
import type { Terminal } from '@xterm/xterm';

// A minimal mutable holder, compatible with React's MutableRefObject<boolean>.
export interface BooleanRef {
  current: boolean;
}

// Standard Page Up / Page Down key sequences — the same bytes a physical key
// sends (unaffected by application-cursor mode), used to scroll full-screen apps.
export const PAGE_UP = '\x1b[5~';
export const PAGE_DOWN = '\x1b[6~';

/**
 * Scroll the terminal by whole pages, picking the right mechanism for the
 * current screen:
 *
 * - Alternate screen (a full-screen app like Claude Code, vim, less, tmux): the
 *   app owns the display and xterm keeps no scrollback here, so send Page Up /
 *   Page Down to the session and let the app scroll its own history.
 * - Normal screen (a plain shell): scroll xterm's own scrollback buffer, and
 *   keep the "user has scrolled up" flag (which pauses auto-scroll-on-output)
 *   consistent with the wheel handler.
 *
 * `delta` is in pages: -1 = one screenful up, +1 = one screenful down.
 */
export function scrollTerminalPages(
  term: Terminal,
  delta: number,
  userScrolled: BooleanRef,
  sendInput: (data: string) => void,
): void {
  if (delta === 0) return;
  if (term.buffer.active.type === 'alternate') {
    sendInput(delta < 0 ? PAGE_UP : PAGE_DOWN);
    return;
  }
  term.scrollPages(delta);
  if (delta < 0) {
    userScrolled.current = true;
  } else if (term.buffer.active.viewportY >= term.buffer.active.baseY) {
    userScrolled.current = false;
  }
}
