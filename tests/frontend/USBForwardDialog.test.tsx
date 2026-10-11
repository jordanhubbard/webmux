import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { USBForwardDialog } from '@frontend/components/USBForwardDialog';

describe('USB redirection setup', () => {
  it('copies a local command and explains the native boundary', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    render(<USBForwardDialog onClose={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'dev@build-host' } });
    fireEvent.change(screen.getByLabelText('SSH port'), { target: { value: '2222' } });
    fireEvent.change(screen.getByLabelText('Local exporter port'), { target: { value: '3240' } });
    fireEvent.change(screen.getByLabelText('Lease'), { target: { value: '60' } });
    fireEvent.click(screen.getByRole('button', { name: 'Copy local command' }));
    expect(writeText).toHaveBeenCalledWith('webmux usb-forward --host dev@build-host --ssh-port 2222 --export-port 3240 --duration 60m');
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Command copied'));
    expect(screen.getByText(/does not start a tunnel or monitor devices/)).toBeInTheDocument();
  });
  it.each(['-oProxyCommand=evil', 'host;whoami', '$(id)', 'user@host & calc', 'host%PATH%', 'host`id`'])('rejects unsafe destination %s', host => {
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: host } });
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
    expect(screen.getByLabelText('USB forwarding command')).toHaveValue('');
  });
  it.each(['0', '1023', '65536', '1e4', '3240;id'])('rejects invalid exporter port %s', port => {
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'build-host' } });
    fireEvent.change(screen.getByLabelText('Local exporter port'), { target: { value: port } });
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
  });
  it('keeps the command selectable when clipboard access fails', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } });
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'build-host' } });
    fireEvent.click(screen.getByRole('button', { name: 'Copy local command' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Clipboard unavailable'));
    expect((screen.getByLabelText('USB forwarding command') as HTMLTextAreaElement).value).toContain('--host build-host');
  });
});
