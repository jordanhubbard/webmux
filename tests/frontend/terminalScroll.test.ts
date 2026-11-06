import { describe, expect, it, vi } from 'vitest';
import type { Terminal } from '@xterm/xterm';
import { scrollTerminalPages, PAGE_UP, PAGE_DOWN } from '@frontend/utils/terminalScroll';

// Minimal xterm stub exposing only what scrollTerminalPages touches.
function stubTerm(type: 'normal' | 'alternate', viewportY: number, baseY: number) {
  return {
    scrollPages: vi.fn(),
    buffer: { active: { type, viewportY, baseY } },
  } as unknown as Terminal;
}

describe('scrollTerminalPages — alternate screen (full-screen app)', () => {
  it('sends Page Up instead of scrolling xterm when scrolling up', () => {
    const term = stubTerm('alternate', 0, 0);
    const sendInput = vi.fn();
    const userScrolled = { current: false };
    scrollTerminalPages(term, -1, userScrolled, sendInput);
    expect(sendInput).toHaveBeenCalledWith(PAGE_UP);
    expect(term.scrollPages).not.toHaveBeenCalled();
  });

  it('sends Page Down when scrolling down', () => {
    const term = stubTerm('alternate', 0, 0);
    const sendInput = vi.fn();
    scrollTerminalPages(term, 1, { current: false }, sendInput);
    expect(sendInput).toHaveBeenCalledWith(PAGE_DOWN);
    expect(term.scrollPages).not.toHaveBeenCalled();
  });
});

describe('scrollTerminalPages — normal screen (plain shell)', () => {
  it('scrolls xterm up by whole pages and marks the view as user-controlled', () => {
    const term = stubTerm('normal', 100, 100);
    const sendInput = vi.fn();
    const userScrolled = { current: false };
    scrollTerminalPages(term, -1, userScrolled, sendInput);
    expect(term.scrollPages).toHaveBeenCalledWith(-1);
    expect(userScrolled.current).toBe(true);
    expect(sendInput).not.toHaveBeenCalled();
  });

  it('keeps the user-controlled flag set when scrolling down but still above the base', () => {
    const term = stubTerm('normal', 40, 100);
    const userScrolled = { current: true };
    scrollTerminalPages(term, 1, userScrolled, vi.fn());
    expect(term.scrollPages).toHaveBeenCalledWith(1);
    expect(userScrolled.current).toBe(true);
  });

  it('clears the user-controlled flag once scrolled back to the bottom', () => {
    const term = stubTerm('normal', 100, 100);
    const userScrolled = { current: true };
    scrollTerminalPages(term, 1, userScrolled, vi.fn());
    expect(term.scrollPages).toHaveBeenCalledWith(1);
    expect(userScrolled.current).toBe(false);
  });

  it('does nothing for a zero delta', () => {
    const term = stubTerm('normal', 100, 100);
    const sendInput = vi.fn();
    const userScrolled = { current: false };
    scrollTerminalPages(term, 0, userScrolled, sendInput);
    expect(term.scrollPages).not.toHaveBeenCalled();
    expect(sendInput).not.toHaveBeenCalled();
    expect(userScrolled.current).toBe(false);
  });
});
