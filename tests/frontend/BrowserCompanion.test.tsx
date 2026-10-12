import { it, expect, vi } from 'vitest';
import { render, act, screen } from '@testing-library/react';
import { BrowserCompanion } from '@frontend/components/BrowserCompanion';
import type { Session } from '@frontend/types';
const fixture = vi.hoisted(() => ({ action: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { browserAction: fixture.action } }));
it.each(['frame', 'request'])('keeps polling after a transient %s failure during navigation', async failure => {
  vi.useFakeTimers();
  try {
    const frame = { image: 'fixture', url: 'https://example.com/start', tabs: [], target: 'one' };
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
