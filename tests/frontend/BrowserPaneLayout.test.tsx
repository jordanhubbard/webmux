import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { BrowserCompanion } from '@frontend/components/BrowserCompanion';
import { BROWSER_PANE_LAYOUT_KEY, parseBrowserPaneLayout } from '@frontend/utils/browserPaneLayout';
import type { Session } from '@frontend/types';

const fixture = vi.hoisted(() => ({ action: vi.fn(), release: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { browserAction: fixture.action, releaseBrowser: fixture.release,
  getBrowserProfiles: vi.fn().mockResolvedValue([]), endBrowser: vi.fn().mockResolvedValue(undefined) } }));
class LayoutPointerEvent extends MouseEvent {
  readonly pointerId: number; readonly isPrimary: boolean;
  constructor(name: string, init: PointerEventInit = {}) { super(name, init); this.pointerId = init.pointerId ?? 1; this.isPrimary = init.isPrimary ?? true; }
}
const frame = { protocol: 3, control: { owned: true, available: false, token: 'private-control-token' },
  image: 'fixture', url: 'https://private.example/page', target: 'private-target', profileId: 'private-profile', tabs: [] };
const session = { id: 'private-terminal', title: 'Fixture', transport: 'local' } as Session;
const pane = () => screen.getByRole('complementary', { name: 'Browser for Fixture' });
const number = (property: 'left' | 'top' | 'width' | 'height') => parseFloat(pane().style[property]);
const geometry = () => ({ left: pane().style.left, top: pane().style.top, width: pane().style.width, height: pane().style.height });
async function open() {
  const view = render(<BrowserCompanion session={session} onHide={vi.fn()} />);
  await act(async () => { await Promise.resolve(); }); return view;
}
function viewport(width: number, height: number) {
  act(() => { vi.stubGlobal('innerWidth', width); vi.stubGlobal('innerHeight', height); window.dispatchEvent(new Event('resize')); });
}
beforeEach(() => {
  localStorage.removeItem(BROWSER_PANE_LAYOUT_KEY); vi.useFakeTimers();
  vi.stubGlobal('innerWidth', 1280); vi.stubGlobal('innerHeight', 900); vi.stubGlobal('PointerEvent', LayoutPointerEvent);
  fixture.action.mockReset().mockResolvedValue(frame); fixture.release.mockReset().mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); localStorage.removeItem(BROWSER_PANE_LAYOUT_KEY); });

it('remembers dock width through expand, immediate unmount and reopen without sending layout gestures to the worker', async () => {
  const view = await open(); expect(number('width')).toBe(832);
  fireEvent.keyDown(screen.getByRole('separator', { name: 'Resize browser width' }), { key: 'ArrowLeft', shiftKey: true });
  expect(number('width')).toBe(882);
  fireEvent.click(screen.getByRole('button', { name: 'Expand browser' })); expect(pane().style.width).toBe('');
  expect(screen.queryByRole('separator', { name: 'Resize browser width' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Restore browser size' })); expect(number('width')).toBe(882);
  expect(fixture.action.mock.calls.map(call => call[1].action)).toEqual(['start']);
  view.unmount(); expect(parseBrowserPaneLayout(localStorage.getItem(BROWSER_PANE_LAYOUT_KEY)).dockWidth).toBe(882);
  await open(); expect(number('width')).toBe(882);
});

it('keeps floating geometry through docking and expansion with the same mounted browser and page', async () => {
  await open(); fireEvent.click(screen.getByRole('button', { name: 'Undock browser' }));
  fireEvent.keyDown(screen.getByRole('button', { name: 'Move browser pane' }), { key: 'ArrowRight' });
  fireEvent.keyDown(screen.getByRole('button', { name: 'Move browser pane' }), { key: 'ArrowDown', shiftKey: true });
  fireEvent.keyDown(screen.getByRole('button', { name: 'Resize browser pane' }), { key: 'ArrowRight' });
  fireEvent.keyDown(screen.getByRole('button', { name: 'Resize browser pane' }), { key: 'ArrowDown' });
  const original = geometry();
  fireEvent.click(screen.getByRole('button', { name: 'Dock browser' })); expect(number('width')).toBe(832);
  fireEvent.click(screen.getByRole('button', { name: 'Undock browser' })); expect(geometry()).toEqual(original);
  fireEvent.click(screen.getByRole('button', { name: 'Expand browser' })); expect(screen.getByRole('button', { name: 'Dock browser' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Restore browser size' })); expect(geometry()).toEqual(original);
  expect(fixture.action.mock.calls.map(call => call[1].action)).toEqual(['start']); expect(fixture.release).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Remote browser URL')).toHaveValue(frame.url);
});

it('preserves desktop geometry across mobile and persists only layout, not page, session, profile or controller identities', async () => {
  const view = await open(); fireEvent.click(screen.getByRole('button', { name: 'Undock browser' }));
  fireEvent.keyDown(screen.getByRole('button', { name: 'Move browser pane' }), { key: 'ArrowRight' });
  act(() => { vi.advanceTimersByTime(120); });
  const stored = localStorage.getItem(BROWSER_PANE_LAYOUT_KEY)!;
  expect(Object.keys(JSON.parse(stored)).sort()).toEqual(['dockWidth', 'floating', 'mode', 'version']);
  for (const value of [session.id, frame.url, frame.target, frame.profileId, frame.control.token]) expect(stored).not.toContain(value);
  const original = geometry(); viewport(320, 568);
  expect(screen.queryByRole('button', { name: 'Dock browser' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Move browser pane' })).toBeNull(); expect(screen.queryByRole('button', { name: 'Resize browser pane' })).toBeNull();
  expect(geometry()).toEqual({ left: '', top: '', width: '', height: '' });
  viewport(1280, 900); expect(geometry()).toEqual(original);
  view.unmount(); expect(localStorage.getItem(BROWSER_PANE_LAYOUT_KEY)).toBe(stored);
  await open(); expect(geometry()).toEqual(original);
});

it('bounds pointer moves and resizes, refuses a different pointer and stops canceled gestures', async () => {
  await open(); fireEvent.click(screen.getByRole('button', { name: 'Undock browser' }));
  const move = screen.getByRole('button', { name: 'Move browser pane' }); const original = geometry();
  fireEvent.pointerDown(move, { pointerId: 1, button: 0, buttons: 1, clientX: 400, clientY: 100 });
  fireEvent.pointerMove(move, { pointerId: 2, buttons: 1, clientX: -5000, clientY: -5000 }); expect(geometry()).toEqual(original);
  fireEvent.pointerMove(move, { pointerId: 1, buttons: 1, clientX: -5000, clientY: -5000 }); expect(number('left')).toBe(8); expect(number('top')).toBe(64);
  fireEvent.pointerCancel(move, { pointerId: 1 });
  fireEvent.pointerMove(move, { pointerId: 1, buttons: 1, clientX: 9000, clientY: 9000 }); expect(number('left')).toBe(8); expect(number('top')).toBe(64);
  const resize = screen.getByRole('button', { name: 'Resize browser pane' });
  fireEvent.pointerDown(resize, { pointerId: 3, button: 0, buttons: 1, clientX: 800, clientY: 600 });
  fireEvent.pointerMove(resize, { pointerId: 3, buttons: 1, clientX: 9000, clientY: 9000 }); expect(number('width')).toBe(1264); expect(number('height')).toBe(828);
  fireEvent.pointerUp(resize, { pointerId: 3 });
});

it('uses safe defaults for malformed or denied storage and keeps controls usable', async () => {
  localStorage.setItem(BROWSER_PANE_LAYOUT_KEY, 'x'.repeat(1025)); const view = await open(); expect(number('width')).toBe(832); view.unmount();
  const denied: Storage = { get length() { return 0; }, getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); }, removeItem() {}, clear() {}, key() { return null; } };
  vi.stubGlobal('localStorage', denied); await open(); fireEvent.click(screen.getByRole('button', { name: 'Undock browser' }));
  fireEvent.keyDown(screen.getByRole('button', { name: 'Move browser pane' }), { key: 'ArrowRight' }); act(() => { vi.advanceTimersByTime(120); });
  expect(pane()).toHaveClass('browser-floating'); expect(screen.queryByRole('alert')).toBeNull();
});

it('lets a view-only viewer arrange the pane without a claim or page input', async () => {
  fixture.action.mockResolvedValue({ ...frame, control: { owned: false, available: false } }); await open();
  fireEvent.click(screen.getByRole('button', { name: 'Undock browser' })); fireEvent.keyDown(screen.getByRole('button', { name: 'Move browser pane' }), { key: 'ArrowRight' });
  fireEvent.keyDown(screen.getByRole('button', { name: 'Resize browser pane' }), { key: 'ArrowRight' });
  expect(screen.getByRole('status')).toHaveTextContent('View only'); expect(fixture.action.mock.calls.map(call => call[1].action)).toEqual(['start']);
});

it('raises the focused companion while leaving the other session mounted', async () => {
  const one = render(<BrowserCompanion session={{ ...session, title: 'One' }} onHide={vi.fn()} />);
  const two = render(<BrowserCompanion session={{ ...session, id: 'other', title: 'Two' }} onHide={vi.fn()} />);
  await act(async () => { await Promise.resolve(); });
  const first = screen.getByRole('complementary', { name: 'Browser for One' }); const second = screen.getByRole('complementary', { name: 'Browser for Two' });
  fireEvent.focus(within(first).getByRole('button', { name: 'Undock browser' }));
  expect(first.parentElement?.style.zIndex).toBe('902'); expect(second.parentElement?.style.zIndex).toBe('901');
  expect(fixture.action.mock.calls.filter(call => call[1].action === 'start')).toHaveLength(2); one.unmount(); two.unmount();
});
