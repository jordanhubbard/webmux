import { afterEach, describe, expect, it, vi } from 'vitest';
import { installGridWheel } from '@frontend/utils/gridWheel';

const cleanups: Array<() => void> = [];
afterEach(() => { cleanups.splice(0).forEach(cleanup => cleanup()); document.body.innerHTML = ''; });
function fixture() {
  const root = document.createElement('div');
  root.innerHTML = '<div data-terminal-session="one"><div class="xterm-screen"></div></div><div data-terminal-session="two"><div class="xterm-screen"></div></div><button>Control</button>';
  document.body.append(root);
  const screens = Array.from(root.querySelectorAll<HTMLElement>('.xterm-screen'));
  const received = [vi.fn(), vi.fn()];
  screens.forEach((screen, index) => screen.addEventListener('wheel', received[index]));
  let selected: string | null = 'one';
  const cleanup = installGridWheel(root, () => selected);
  cleanups.push(cleanup);
  const wheel = (target: HTMLElement, options: WheelEventInit = {}) => {
    const event = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaX: 8, deltaY: 24, ...options });
    target.dispatchEvent(event); return event;
  };
  return { root, screens, received, wheel, cleanup, select: (id: string | null) => { selected = id; } };
}
describe('grid wheel ownership', () => {
  it('routes once to the selected window over another tile, preserving fractional deltas', () => {
    const f = fixture();
    const original = f.wheel(f.screens[1], { deltaX: .25, deltaY: -2.5 });
    expect(f.received[0]).toHaveBeenCalledTimes(1);
    expect(f.received[1]).not.toHaveBeenCalled();
    expect(f.received[0].mock.calls[0][0]).toMatchObject({ deltaX: .25, deltaY: -2.5 });
    expect(original.defaultPrevented).toBe(true);
    expect([f.root.scrollLeft, f.root.scrollTop]).toEqual([0, 0]);
  });
  it('routes wheel over add-session buttons to the selected window', () => {
    const f = fixture();
    const event = f.wheel(f.root.querySelector('button')!);
    expect(f.received[0]).toHaveBeenCalledTimes(1);
    expect(f.received[1]).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(true);
    expect([f.root.scrollLeft, f.root.scrollTop]).toEqual([0, 0]);
  });
  it('preserves native legacy deltas used by terminal wheel normalization', () => {
    const f = fixture();
    const event = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: -240 });
    Object.defineProperty(event, 'wheelDeltaY', { value: 120 });
    f.screens[1].dispatchEvent(event);
    expect(Reflect.get(f.received[0].mock.calls[0][0], 'wheelDeltaY')).toBe(120);
  });
  it('preserves native window handling and then prevents ancestor scroll chaining', () => {
    const f = fixture(); let preventedAtWindow = true;
    f.screens[0].addEventListener('wheel', event => { preventedAtWindow = event.defaultPrevented; });
    const event = f.wheel(f.screens[0]);
    expect(f.received[0]).toHaveBeenCalledTimes(1);
    expect(preventedAtWindow).toBe(false);
    expect(event.defaultPrevented).toBe(true);
  });
  it('Shift pans both grid axes before any application receives the gesture', () => {
    const f = fixture(); const event = f.wheel(f.screens[0], { shiftKey: true });
    expect([f.root.scrollLeft, f.root.scrollTop]).toEqual([8, 24]);
    expect(f.received[0]).not.toHaveBeenCalled();
    expect(f.received[1]).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(true);
    f.wheel(f.root.querySelector('button')!, { shiftKey: true, deltaMode: 1 });
    expect([f.root.scrollLeft, f.root.scrollTop]).toEqual([136, 408]);
  });
  it('uses the selected window from a grid gap and follows selection changes', () => {
    const f = fixture(); f.select('two'); f.wheel(f.root);
    expect(f.received[0]).not.toHaveBeenCalled(); expect(f.received[1]).toHaveBeenCalledTimes(1);
    f.select(null); f.wheel(f.screens[0]);
    expect(f.received[0]).toHaveBeenCalledTimes(1);
  });
  it('keeps native embedded-content scrolling local while still containing the grid', () => {
    const f = fixture(); const embedded = document.createElement('canvas');
    embedded.setAttribute('data-wheel-native', ''); f.root.append(embedded);
    const wheel = vi.fn(); embedded.addEventListener('wheel', wheel);
    expect(f.wheel(embedded).defaultPrevented).toBe(true);
    expect(wheel).toHaveBeenCalledTimes(1); expect(f.received[0]).not.toHaveBeenCalled();
    f.wheel(embedded, { shiftKey: true });
    expect(wheel).toHaveBeenCalledTimes(1); expect(f.root.scrollTop).toBe(24);
  });
  it('routes desktop surfaces through their native wheel handler with exclusive Shift ownership', () => {
    const f = fixture(); const desktop = document.createElement('div');
    desktop.dataset.desktopSession = 'remote';
    desktop.innerHTML = '<div data-wheel-surface><canvas></canvas></div>';
    f.root.append(desktop);
    const surface = desktop.querySelector<HTMLElement>('[data-wheel-surface]')!;
    const received = vi.fn(); surface.addEventListener('wheel', received);
    f.select('remote'); f.wheel(f.screens[0]);
    expect(received).toHaveBeenCalledTimes(1); expect(f.received[0]).not.toHaveBeenCalled();
    f.wheel(f.root, { shiftKey: true });
    expect(received).toHaveBeenCalledTimes(1);
    expect([f.root.scrollLeft, f.root.scrollTop]).toEqual([8, 24]);
    f.select('one'); f.wheel(surface);
    expect(f.received[0]).toHaveBeenCalledTimes(1); expect(received).toHaveBeenCalledTimes(1);
  });
  it('preserves browser zoom and other modified input, and removes listeners', () => {
    const f = fixture(); const event = f.wheel(f.screens[1], { ctrlKey: true });
    expect(event.defaultPrevented).toBe(false);
    expect(f.received[0]).not.toHaveBeenCalled(); expect(f.received[1]).toHaveBeenCalledTimes(1);
    f.cleanup(); expect(f.wheel(f.root, { shiftKey: true }).defaultPrevented).toBe(false);
  });
});
