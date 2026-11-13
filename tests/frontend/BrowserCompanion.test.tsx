import { it, expect, vi } from 'vitest';
import { render, act, screen, fireEvent } from '@testing-library/react';
import { BrowserCompanion } from '@frontend/components/BrowserCompanion';
import type { Session } from '@frontend/types';
const fixture = vi.hoisted(() => ({ action: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { browserAction: fixture.action } }));

it.each(['about:blank', 'https://example.com/previous'])('keeps a URL draft when a delayed %s frame arrives between blur and Go', async previous => {
 vi.useFakeTimers();
 const frame = { protocol: 2, image: 'fixture', url: previous, tabs: [], target: 'one' };
 const destination = 'http://localhost:3000/authorize';
 const redirect = 'https://provider.example/redirect';
 let resolveFrame!: (value: typeof frame) => void;
 fixture.action.mockReset().mockResolvedValueOnce(frame)
  .mockImplementationOnce(() => new Promise(resolve => { resolveFrame = resolve; }))
  .mockResolvedValue({ ...frame, url: redirect });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  await act(async () => { vi.advanceTimersByTime(500); });
  expect(fixture.action).toHaveBeenLastCalledWith('test', { action: 'frame' });
  const address = screen.getByLabelText('Remote browser URL');
  fireEvent.focus(address);
  fireEvent.change(address, { target: { value: destination } });
  // Clicking Go blurs the address before the form submits. A completed poll
  // in that interval must not replace the URL that the user is submitting.
  fireEvent.blur(address);
  await act(async () => { resolveFrame(frame); });
  expect(address).toHaveValue(destination);
  fireEvent.click(screen.getByRole('button', { name: 'Go' }));
  await act(async () => { await Promise.resolve(); });
  expect(fixture.action).toHaveBeenCalledWith('test', { action: 'navigate', url: destination });
  expect(address).toHaveValue(redirect);
 } finally { view.unmount(); vi.useRealTimers(); }
});

it('releases the address draft when the user explicitly navigates back', async () => {
 const frame = { protocol: 2, image: 'fixture', url: 'https://example.com/work', canBack: true, tabs: [], target: 'one' };
 fixture.action.mockReset().mockResolvedValueOnce(frame).mockResolvedValue({ ...frame, url: 'https://example.com/previous' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  const address = screen.getByLabelText('Remote browser URL');
  fireEvent.focus(address);
  fireEvent.change(address, { target: { value: 'https://typed.example/draft' } });
  fireEvent.blur(address);
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  await act(async () => { await Promise.resolve(); });
  expect(address).toHaveValue('https://example.com/previous');
  expect(fixture.action).toHaveBeenCalledWith('test', { action: 'back' });
 } finally { view.unmount(); }
});

it.each(['about:blank', 'https://example.com/work'])('Escape cancels a URL draft and resumes address updates for %s', async url => {
 vi.useFakeTimers();
 const frame = { protocol: 2, image: 'fixture', url, tabs: [], target: 'one' };
 fixture.action.mockReset().mockResolvedValueOnce(frame).mockResolvedValue({ ...frame, url: 'https://example.com/redirect' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  const address = screen.getByLabelText('Remote browser URL');
  fireEvent.focus(address);
  fireEvent.change(address, { target: { value: 'https://typed.example/draft' } });
  fireEvent.keyDown(address, { key: 'Escape' });
  expect(address).toHaveValue(url === 'about:blank' ? '' : url);
  expect(fixture.action).toHaveBeenCalledTimes(1);
  await act(async () => { vi.advanceTimersByTime(500); });
  expect(address).toHaveValue('https://example.com/redirect');
 } finally { view.unmount(); vi.useRealTimers(); }
});

it('does not navigate when Go is pressed with an empty address', async () => {
 fixture.action.mockReset().mockResolvedValue({ protocol: 2, image: 'fixture', url: 'about:blank', tabs: [], target: 'one' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  fireEvent.click(screen.getByRole('button', { name: 'Go' }));
  await act(async () => { await Promise.resolve(); });
  expect(fixture.action.mock.calls.map(call => call[1].action)).toEqual(['start']);
 } finally { view.unmount(); }
});

it.each(['frame', 'request'])('keeps polling after a transient %s failure during navigation', async failure => {
  vi.useFakeTimers();
  try {
    const frame = { protocol: 2, image: 'fixture', url: 'https://example.com/start', tabs: [], target: 'one' };
    fixture.action.mockReset().mockResolvedValueOnce(frame);
    if (failure === 'frame') fixture.action.mockResolvedValueOnce({ error: 'Navigation interrupted screenshot' });
    else fixture.action.mockRejectedValueOnce(new Error('Navigation interrupted request'));
    fixture.action.mockResolvedValue({ ...frame, url: 'https://example.com/callback' });
    const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
    await act(async () => { await Promise.resolve(); });
    await act(async () => { vi.advanceTimersByTime(1000); });
    expect(screen.getByRole('alert').textContent).toContain('Navigation');
    await act(async () => { vi.advanceTimersByTime(1000); });
    expect(screen.getByLabelText('Remote browser URL')).toHaveValue('https://example.com/callback');
    expect(screen.queryByRole('alert')).toBeNull();
    view.unmount();
  } finally { vi.useRealTimers(); }
});

it.each([
 ['https://example.com/work', true],
 ['http://localhost:3000/callback', false],
 ['https://example.com/auth?redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback&state=opaque', false],
])('offers the correct device-browser path for %s', async (url, direct) => {
 const handoff = vi.fn();
 fixture.action.mockReset().mockResolvedValue({ protocol: 2, image: 'fixture', url, tabs: [{ id: 'one', title: 'Page' }], target: 'one' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} onAuthentication={handoff} />);
 try {
  await act(async () => { await Promise.resolve(); });
  const link = screen.queryByRole('link', { name: 'Open in your browser' });
  if (direct) { expect(link).toHaveAttribute('href', url); expect(link).toHaveAttribute('rel', 'noopener noreferrer'); }
  else expect(link).toBeNull();
  const { fireEvent } = await import('@testing-library/react');
  fireEvent.click(screen.getByRole('button', { name: 'Sign-in handoff' }));
  expect(handoff).toHaveBeenCalledWith(url);
 } finally { view.unmount(); }
});

it('sends composed text once rather than forwarding intermediate IME keystrokes', async () => {
 fixture.action.mockReset().mockResolvedValue({ protocol: 2, image: 'fixture', url: 'https://example.com', tabs: [], target: 'one' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'local' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  const { fireEvent } = await import('@testing-library/react');
  const input = screen.getByLabelText('Type into remote page');
  await act(async () => {
   fireEvent.compositionStart(input);
   fireEvent.keyDown(input, { key: 'Process', isComposing: true });
   fireEvent.input(input, { target: { value: '日本語' } });
   fireEvent.compositionEnd(input, { data: '日本語' });
   fireEvent.input(input);
   await Promise.resolve();
  });
  const sent = fixture.action.mock.calls.map(call => call[1]).filter(action => action.action === 'text' || action.action === 'key');
  expect(sent).toEqual([{ action: 'text', text: '日本語' }]);
 } finally { view.unmount(); }
});

it('reports an older terminal-host worker instead of silently sending unsupported controls', async () => {
 fixture.action.mockReset().mockResolvedValue({ image: 'fixture', url: 'https://example.com', tabs: [], target: 'one' });
 const view = render(<BrowserCompanion session={{ id: 'test', title: 'test', transport: 'ssh' } as Session} onHide={vi.fn()} />);
 try {
  await act(async () => { await Promise.resolve(); });
  expect(screen.getByRole('alert')).toHaveTextContent('Update WebMux on the terminal host');
  expect(screen.queryByAltText('Interactive remote browser viewport')).toBeNull();
 } finally { view.unmount(); }
});
