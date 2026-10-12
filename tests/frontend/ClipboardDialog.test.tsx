import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ClipboardDialog } from '@frontend/components/ClipboardDialog';

afterEach(() => { vi.restoreAllMocks(); Reflect.deleteProperty(navigator, 'clipboard'); });
describe('explicit clipboard access', () => {
  it('offers manual paste when the browser has no clipboard API', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    const send = vi.fn();
    render(<ClipboardDialog onSend={send} onClose={vi.fn()} />);
    fireEvent.click(screen.getByText('Read clipboard'));
    expect(await screen.findByText(/unavailable or denied/)).toBeDefined();
    fireEvent.change(screen.getByLabelText('Clipboard text'), { target: { value: 'hello 🔐' } });
    fireEvent.click(screen.getByText('Send text'));
    expect(send).toHaveBeenCalledWith('hello 🔐');
  });
  it('does not access the clipboard until clicked and reports denial', async () => {
    const readText = vi.fn().mockRejectedValue(new Error('denied'));
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { readText } });
    render(<ClipboardDialog initialText="remote text" onSend={vi.fn()} onClose={vi.fn()} />);
    expect(readText).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('Read clipboard'));
    await waitFor(() => expect(screen.getByRole('status').textContent).toMatch(/unavailable or denied/));
    expect(screen.getByLabelText('Clipboard text')).toHaveValue('remote text');
  });
  it('rejects oversized UTF-8 before sending', () => {
    const send = vi.fn();
    render(<ClipboardDialog initialText={'🔐'.repeat(40000)} onSend={send} onClose={vi.fn()} />);
    fireEvent.click(screen.getByText('Send text'));
    expect(send).not.toHaveBeenCalled();
    expect(screen.getByRole('status').textContent).toMatch(/128 KiB/);
  });
});
