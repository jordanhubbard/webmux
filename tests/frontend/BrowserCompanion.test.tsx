import { it, expect, vi } from 'vitest';
import { render, act, screen } from '@testing-library/react';
import { BrowserCompanion } from '@frontend/components/BrowserCompanion';
import type { Session } from '@frontend/types';
const fixture = vi.hoisted(() => ({ action: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { browserAction: fixture.action } }));
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
