import { StrictMode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Dialog } from '@frontend/components/Dialog';

describe('Dialog', () => {
  it('has an accessible title and allows native cancel to request dismissal', () => {
    const onClose = vi.fn();
    render(<Dialog title="Connect" onClose={onClose}><input aria-label="Host" /></Dialog>);
    const dialog = screen.getByRole('dialog', { name: 'Connect' });
    expect(dialog).toHaveAttribute('open');
    const cancel = new Event('cancel', { cancelable: true });
    fireEvent(dialog, cancel);
    expect(cancel.defaultPrevented).toBe(true);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('only dismisses a gesture that begins and ends on the backdrop', () => {
    const onClose = vi.fn();
    render(<Dialog title="Connect" onClose={onClose}><input aria-label="Host" /></Dialog>);
    const dialog = screen.getByRole('dialog');
    fireEvent.pointerDown(screen.getByLabelText('Host'));
    fireEvent.click(dialog);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.pointerDown(dialog);
    fireEvent.click(dialog);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('guards all dismissal routes until the pending operation completes', () => {
    const onClose = vi.fn();
    const { rerender } = render(<Dialog title="Settings" onClose={onClose} dismissible={false}>Saving…</Dialog>);
    const dialog = screen.getByRole('dialog');
    expect(screen.getByRole('button', { name: 'Close settings' })).toBeDisabled();
    fireEvent(dialog, new Event('cancel', { cancelable: true }));
    fireEvent.pointerDown(dialog);
    fireEvent.click(dialog);
    expect(onClose).not.toHaveBeenCalled();
    rerender(<Dialog title="Settings" onClose={onClose}>Saved</Dialog>);
    fireEvent.click(screen.getByRole('button', { name: 'Close settings' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('supports a non-dismissible session-expiry dialog', () => {
    render(<Dialog title="Session timed out" dismissible={false}><button>Sign in</button></Dialog>);
    expect(screen.queryByRole('button', { name: /Close/ })).not.toBeInTheDocument();
    const dialog = screen.getByRole('dialog');
    fireEvent(dialog, new Event('cancel', { cancelable: true }));
    expect(dialog).toHaveAttribute('open');
  });

  it('restores the opener on unmount, including StrictMode effect replay', () => {
    const opener = document.createElement('button');
    document.body.append(opener);
    opener.focus();
    const { unmount } = render(<StrictMode><Dialog title="Help" onClose={vi.fn()}><button>Done</button></Dialog></StrictMode>);
    screen.getByRole('button', { name: 'Done' }).focus();
    unmount();
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
